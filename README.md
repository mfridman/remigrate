`remigrate` is a tool for creating a database, tables and secondary indexes in rethinkDB.

More info on the database can be found here: [RethinkDB](https://www.rethinkdb.com/docs) and the underlying driver can be found here: [gorethink](https://github.com/GoRethink/gorethink)

## Installation

    go get -u github.com/mfridman/remigrate

## Usage

`remigrate` reads from a file named config (by default) in the same directory or an absolute path specified with the `--config` flag. File format must be YAML, the extension .yml or .yaml can be omitted.

    remigrate --config /etc/configs/rethinkdb_up.yml

A sample YAML file, config, is included in the Git repo.

__Note__, only simple indexes are supported for now. If you need geospatial index file an issue.

```yaml
# mandatory options
ip: localhost
port: 28015 # this is the default rethinkdb port
database_name: machines

tables:
  - name: robots # mandatory
    primary_key: serial_num # can be omitted, defaults to id
    simple_index: [version, model] # can be omitted
  - name: parts # mandatory
```

Output:

```shell
[machines                      ] create     database
[robots                        ] create     table
[version                       ] create     secondary index on robots
[model                         ] create     secondary index on robots
[parts                         ] create     table
---
1   database created
2   table(s) created
2   secondary index(es) created
```

Running the above will create a machines database, with 2 tables: robots and features. The robots table primary key is serial_num and secondary indexes are version and model.

You can read more here: [Using secondary indexes in RethinkDB](https://www.rethinkdb.com/docs/secondary-indexes)

Re-running the above file is safe, i.e., existing items are ignored:

```shell
[machines                      ] ignore     database exists
[robots                        ] ignore     table exists
[version                       ] ignore     secondary index exists on robots
[model                         ] ignore     secondary index exists on robots
[parts                         ] ignore     table exists
---
0   database created
0   table(s) created
0   secondary index(es) created
```

The table info for the above example in rethinkdb would be:

robots:

```json
{

    "db": {
        "id": "7277457e-b653-436f-be02-040cb230e414" ,
        "name": "machines" ,
        "type": "DB"
    } ,
    "doc_count_estimates": [
        18734
    ] ,
    "id": "a42215be-f7b2-42e6-8bd7-d60d20a35afa" ,
    "indexes": [
        "model" ,
        "version"
    ] ,
    "name": "robots" ,
    "primary_key": "serial_num" ,
    "type": "TABLE"

}
```

parts:

```json
{

    "db": {
        "id": "7277457e-b653-436f-be02-040cb230e414" ,
        "name": "machines" ,
        "type": "DB"
    } ,
    "doc_count_estimates": [
        0
    ] ,
    "id": "1bdbb34b-631e-478a-bb2b-4bd35633864e" ,
    "indexes": [ ],
    "name": "parts" ,
    "primary_key": "id" ,
    "type": "TABLE"

}
```