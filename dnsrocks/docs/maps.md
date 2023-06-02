# How maps work
Let's take this example data file:

```
Mwww.foo.com,rw
M*.foo.com,rs
8*.foo.com,ec
%\000\001,10.0.0.0/24,rw
%\000\002,10.0.0.0/24,rs
%\000\002,10.0.0.0/24,ec
+*.foo.com,192.168.5.1,180,3600,,\000\001,1
+*.foo.com,192.127.2.1,180,3600,,\000\002,1
```
1 line at a time....:

`Mwww.foo.com,rw` creates a `resolver IP map (M)` for the domain `www.foo.com` and assign it to the map ID `rw`
`M*.foo.com,rs` creates a `resolver IP map (M)` for the wildcard domain `*.foo.com` and assign it to the map ID `rs`
`8*.foo.com,ec` creates a `ECS map (8)` for the wildcard domain `*.foo.com` and assign it to the map ID `ec`

A resolver IP map is a map that will apply based on the IP making the query to our DNS server.

An ECS map is a map that will apply based on the EDNS Client subnet. This information is typically provided by some public DNS resolvers such as Google DNS and OpenDNS and allows to have a better granularity as to what answer to return.
For privacy reasons only up to /24 for ipv4 and /56 for ipv6 are injected by most resolvers.

Once a map ID is found, the code will try to find a matching location ID by looking for the subnet with the longest prefix that matches the client subnet/resolver ip in the query. This gives the resulting location ID.

`%rw,10.0.0.0/24,\000\001`  would match any queries without ECS from resolvers in range 10.0.0.0/24 asking for www.foo.com to location ID \000\001 while people asking for bar.foo.com would end up in map ID rs and `%rs,10.0.0.0/24,\000\002` and it would be assigned to location ID \000\002

# Handling a request
When a request comes in, first it's verified whether it contains a client subnet, if so, a matching ECS map id is searched for. If  no such map is found (or the default location id is found) a resolver based map will be used.
# Examples
- Let say the server receives a request with client subnet 10.0.0.0/25 from some random resolver ip, asking for www.foo.com
- www.foo.com matches ECS map ec
- For map ec, 10.0.0.0/25 falls into 10.0.0.0/24, so the location \000\002 is used
- The response will be 192.127.2.1
---
- Let say the server receives a request without client subnet from resolver with ip 10.0.0.1, asking for www.foo.com
- www.foo.com matches resolver IP map rw
- For map rw, 10.0.0.1 falls into 10.0.0.0/24, so the location \000\001 is used
- The response will be 192.168.5.1
---
- Let say the server receives a request without client subnet from resolver with ip 10.0.0.1, asking for bar.foo.com
- bar.foo.com matches resolver IP map rs
- For map rs, 10.0.0.1 falls into 10.0.0.0/24, so the location \000\002 is used
- The response will be 192.127.2.1
