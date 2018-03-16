// remigrate is a tool for creating a database, tables and secondary indexes in rethinkDB.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	r "github.com/GoRethink/gorethink"
	"github.com/go-yaml/yaml"
	"github.com/pkg/errors"
)

// VERSION is the current version git tag
var VERSION = "v0.1.0"

const (
	create = "create"
	ignore = "ignore"
)

// Config represents necessary info for establishing a connection to rethinkdb and creating the db structure.
// The database, tables and indexes are created if non-existent.
type Config struct {
	// mandatory options for establishing a connection
	DBIP   string `yaml:"ip"`
	DBPort string `yaml:"port"`

	// DBName must contain alphanumeric characters and underscores
	DBName   string  `yaml:"database_name"`
	DBTables []Table `yaml:"tables"`
}

// A Table represents the metada of a rethinkdb table.
type Table struct {
	// Name represents the name of the table.
	Name string `yaml:"name"`

	// The name of the primary key. If left blank the default primary key is set to id.
	PrimaryKey string `yaml:"primary_key"`

	// Secondary index(es) on a table. Only the "Simple indexes" options is supported for now.
	// https://www.rethinkdb.com/docs/secondary-indexes
	SimpleIndexes []string `yaml:"simple_index"`
}

// global vars to track stats relating to number of dbs, tables and indexes created.
var dbsCreated, tblsCreated, indCreated int

var (
	ver    = flag.Bool("version", false, "prints current version")
	config = flag.String("config", "config", "specify config file path relative to binary, or an absolute path")
	drop   = flag.Bool("dbdrop", false, "drop database specified in config file (CAREFUL !!)")
)

func main() {

	log.SetFlags(0)

	flag.Parse()

	if *ver {
		fmt.Printf("remigrate version: %s\n", VERSION)
		os.Exit(0)
	}

	cfg, err := readInConfig(*config)
	if err != nil {
		log.Fatalln(err)
	}

	session, err := newRethinkSession(cfg)
	if err != nil {
		log.Fatalln(err)
	}
	defer session.Close()

	if !session.IsConnected() {
		log.Fatalln("no connection to rethinkDB")
	}

	// check existence of db by name, create if non-existent under certain conditions.
	// TODO: decouple the *drop and ok logic from dbExists
	ok, err := dbExists(cfg.DBName, session)
	if err != nil {
		log.Fatalln(err)
	}

	switch {
	case !ok && *drop:
		// does not exist and user wants to drop, cannot be done
		log.Fatalf("database [%s] does not exist, cannot drop non-existent database\n", cfg.DBName)
	case !ok:
		// does not exist, create it, move on...
		if err := dbCreate(cfg.DBName, session); err != nil {
			log.Fatalln(err)
		}
		log.Printf("[%-30s] %-10s database\n", cfg.DBName, create)
	case ok && *drop:
		// exists but user wants it deleted
		if ok := confirmDrop(cfg.DBName); !ok {
			log.Printf("exiting without dropping database [%s]\n", cfg.DBName)
			os.Exit(0)
		}
		resp, err := r.DBDrop(cfg.DBName).RunWrite(session)
		if err != nil {
			log.Fatalf("failed to drop [%s] database: %v\n", cfg.DBName, err)
		}
		fmt.Printf("%-3d database dropped\n%-3d table(s) dropped\n", resp.DBsDropped, resp.TablesDropped)
		os.Exit(0)
	case ok:
		// exists, move on...
		log.Printf("[%-30s] %-10s database exists\n", cfg.DBName, ignore)
	}

	// db must exist at this point
	session.Use(cfg.DBName)

	for i := range cfg.DBTables {
		if err := tableUp(cfg.DBTables[i], session); err != nil {
			log.Fatalln(err)
		}
	}

	fmt.Printf("---\n%-3d database created\n%-3d table(s) created\n%-3d secondary index(es) created\n", dbsCreated, tblsCreated, indCreated)
}

func confirmDrop(dbName string) bool {
	bufnr := bufio.NewReader(os.Stdin)
	for i := 3; i > 0; i-- {
		fmt.Printf("are you sure you want to drop the [%s] database [y/n]: ", dbName)
		r, err := bufnr.ReadString('\n')
		if err != nil {
			log.Fatalln(err)
		}
		r = strings.ToLower(strings.TrimSpace(r))
		switch r {
		case "yes", "y":
			return true
		case "no", "n":
			return false
		}
	}
	return false
}

// readInConfig reads config file from the specified location, relative to binary. Can be an absolute or relative path.
// User can overried the default "config" file with the --config option.
func readInConfig(file string) (*Config, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func tableUp(table Table, session *r.Session) error {

	// check table, if non-existent create.
	ok, err := tableExists(table.Name, session)
	if err != nil {
		return err
	} else if !ok {
		if err := tableCreate(table, session); err != nil {
			return err
		}
		log.Printf("[%-30s] %-10s table\n", table.Name, create)
	} else if ok {
		log.Printf("[%-30s] %-10s table exists\n", table.Name, ignore)
	}

	// check secondary simple indexes on table, if non-existent create.
	if err := addSimpleIndexes(table, session); err != nil {
		return err
	}
	return nil
}

func tableCreate(table Table, session *r.Session) error {
	opts := new(r.TableCreateOpts)
	if table.PrimaryKey != "" {
		opts.PrimaryKey = table.PrimaryKey
	}
	resp, err := r.TableCreate(table.Name, *opts).RunWrite(session)
	if err != nil {
		return errors.Wrapf(err, "failed to create [%s] table", table.Name)
	}
	tblsCreated += resp.TablesCreated
	return nil
}

func simpleIndexMap(tblName string, session *r.Session) (map[string]bool, error) {
	cur, err := r.Table(tblName).IndexList().Run(session)
	if err != nil {
		return nil, err
	}
	var indexes []string
	if err := cur.All(&indexes); err != nil {
		return nil, err
	}
	lookup := make(map[string]bool)
	for i := range indexes {
		lookup[indexes[i]] = true
	}
	return lookup, nil
}

func addSimpleIndexes(table Table, session *r.Session) error {
	if len(table.SimpleIndexes) == 0 {
		return nil
	}

	// generate a lookup map of indexes already available in table
	lookup, err := simpleIndexMap(table.Name, session)
	if err != nil {
		return err
	}

	for _, s := range table.SimpleIndexes {
		if _, ok := lookup[s]; ok {
			// index already exists
			log.Printf("[%-30s] %-10s secondary index exists on %s\n", s, ignore, table.Name)
			continue
		}
		if err := indexCreate(table.Name, s, session); err != nil {
			return err
		}
		indCreated++
		log.Printf("[%-30s] %-10s secondary index on %s\n", s, create, table.Name)
	}
	return nil
}

func indexCreate(tblname, index string, session *r.Session) error {
	if _, err := r.Table(tblname).IndexCreate(index).RunWrite(session); err != nil {
		return errors.Wrapf(err, "failed to create [%v] secondary index on table [%v]", index, tblname)
	}
	return nil
}

func newRethinkSession(c *Config) (*r.Session, error) {
	s, err := r.Connect(r.ConnectOpts{Address: c.DBIP + ":" + c.DBPort})
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to rethinkDB")
	}
	r.SetTags("gorethink", "json")
	return s, nil
}

func dbExists(dbName string, session *r.Session) (bool, error) {
	// get currently available database(s)
	cur, err := r.DBList().Run(session)
	if err != nil {
		return false, errors.Wrap(err, "could not list all database names in the system")
	}
	// store db names in slice of strings
	var dbs []string
	if err := cur.All(&dbs); err != nil {
		return false, errors.Wrap(err, "could not store database names in slice of strings")
	}
	for i := range dbs {
		if dbName == dbs[i] {
			return true, nil
		}
	}
	return false, nil
}

func dbCreate(dbName string, session *r.Session) error {
	resp, err := r.DBCreate(dbName).RunWrite(session)
	if err != nil {
		return errors.Wrapf(err, "failed to create [%s] database", dbName)
	}
	// If successful, the command returns an object with two fields, where dbs_created: always 1
	dbsCreated += resp.DBsCreated
	return nil
}

func tableExists(tblName string, session *r.Session) (bool, error) {
	cur, err := r.TableList().Run(session)
	if err != nil {
		return false, errors.Wrap(err, "could not list all table names in database")
	}
	var tbls []string
	if err := cur.All(&tbls); err != nil {
		return false, errors.Wrap(err, "could not store table names in slice of strings")
	}
	for i := range tbls {
		if tblName == tbls[i] {
			return true, nil
		}
	}
	return false, nil
}
