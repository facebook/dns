
# Data format

Our data format is very similar to [TinyDNS's data format](https://cr.yp.to/djbdns/tinydns-data.html) with some very important differences
- Since we support IPv6 we changed the delimiter from `:` to `,`  (although `:` is still a valid delimiter for IPv4 addresses to maintain compatibility with tinydns data format)
- We support Resolver maps in addition to the ECS maps. Resolver map defintions for domains start with `M` similar to how ECS maps start with `8`. For more information on maps read [the documentation on maps](maps.md)

For an example data file, that can be consumed by `dnsrocks-data` look at our [example](https://github.com/facebookincubator/dns/blob/main/dnsrocks/testdata/data/data.in)