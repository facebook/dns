# Backend

DNSRocks supports using both CDB and RocksDB as a backend storage.

Built originally as a drop-in [TinyDNS](https://cr.yp.to/djbdns/tinydns.html) replacement, it evolved to support RocksDB database instead of CDB.

CDB (original storage used in TinyDNS) stands for 'constant database', and thus requires full rebuild of the DB every time there is a change in records.

RocksDB on the other hand is a modern key-value DB that allows applying changes to the existing database.

## Picking backend

For users migrating from TinyDNS, DNSRocks provides a drop-in replacement that can work with CDB compiled with `tinydns-data` out of the box. Provided `dnsrocks-data` cli supports compiling records in our [data_format](data_format.md) to CDB via `-dbdriver=cdb` flag.

### CDB
CDB has few advantages:
* it's extremely simple and fast
* drop-in replacement for TinyDNS, but with IPv6 and map support when needed

It has disadvantages as well:
* constant DB means need for re-compilation for any change in DNS records, consuming resources
* 32-bit format, with 4Gb maximum DB size limit
* no data compression, compiled DB is big

### RocksDB

RocksDB is a full blown modern key-value store, with all the related advantages and disadvantages.

Compared to CDB, it has many advantages, namely:
* Built-in data compression, compiled DB is significantly smaller
* No limit on data size
* Support for dynamic updates to the database (see `dnsrocks-applyrdb` tool)

On the downside though:
* slower key access, as a result slightly lower performance
* harder to tune or reason about
* slower and more resource-intensive DB compilation

## Data key format

When using **RocksDB** as a backend, user can choose v1 or v2 key format. CDB is limited to v1 format only.

Each format has its own benefits, depending on usage pattern and stored records.
We recommend carefully evaluating performance with `dnsperf` against each particular dataset and usage pattern.

### Keys format v1 (default), CDB and RocksDB

* map: `\x00M<domainName>[=/]` or `\x008<domainName>[=/]`. Example: `\x00M\x08facebook\x03com\x00=`
* RRs: `<location><domainName>`. Example: `\x00\x01\x08facebook\x03com\x00`

Domain name is used directly when storing records, leveraging usage of `Get` calls available in any key-value database.

Because of it's simple nature, it is vulnerable to deep-label attacks which could amplify number of DB access has to be performed. DNS spec allows up to 127 labels (255 max domain length), and each label will require a separate key lookup.

### Keys format v2 (RocksDB-only)

Use `dnsrocks-data` with `-dbdriver=rocksdb -useV2Keys` flags to use this format.

* map: `\x00M<reversedDomainName>[=/]` or `\x008<reversedDomainName>[=/]`. Example: `\x00M\x03com\x08facebook\x00=`
* RRs: `\x00o<reversedDomainName><location>`. Example: `\x00o\x03com\x08facebook\x00\x00\x01`

This storage format was designed with RocksDB in mind, leveraging the fact that data is stored in sorted order.

Storing DNS zones in reversed way (like `.com.facebook.www`, and not `www.facebook.com.`) combined with sorted data and RocksDB supporting methods like `SeekPrev` makes it trivial to identify non-existing records without performing `Get` for each label.

This allows to nicely mitigate any deep-label attacks amplifications, but at the cost of slightly slower key lookup.

Also because this format relies on `SeekPrev` RocksDB call which can potentially scan through a range of keys, it's performance is more affected by the DB state. The more updates DB receives between compactions, the more performance degrades.
