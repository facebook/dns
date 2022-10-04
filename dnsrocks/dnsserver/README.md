# tinydnscdb

*tinydnscdb* enables serving DNS data of a [cdb](http://cr.yp.to/cdb.html) file.

## Syntax

```
tinydnscdb CDBFILE [RELOAD_TIME]
```

* **CDBFILE** the CDB database file to read records from.
* **RELOAD_TIME** the interval at which the CDB will be reloaded. 0 do disabled
  automatic reloading. **WARNING**: this is currently disabled...


## Examples

```
. {
    tinydnscdb /var/db/data.cdb 10
}
```


Or to disable reloading

```
. {
    tinydnscdb /var/db/data.cdb
}
```

