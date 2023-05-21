# Data format

The data format is very similar to [TinyDNS's data format](https://cr.yp.to/djbdns/tinydns-data.html) with some very important differences
- Since dnsrocks supports IPv6, delimiter was changed from `:` to `,`  (although `:` is still a valid delimiter for IPv4 addresses to maintain compatibility with tinydns data format)
- dnsrocks supports Resolver IP maps in addition to the ECS maps. Resolver IP map definitions for domains start with `M` similar to how ECS maps start with `8`. For more information on maps read [the documentation on maps](maps.md)

For an example data file that can be consumed by `dnsrocks-data` look at [example](https://github.com/facebookincubator/dns/blob/main/dnsrocks/testdata/data/data.in)
