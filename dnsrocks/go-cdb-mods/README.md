# go-cdb
Go interface to Constant Databases (CDB)

A pure Go reader & writer of the Constant Database format. See
http://cr.yp.to/cdb.html to read more about constant databases.

This was originally a fork of https://github.com/jbarham/go-cdb however
improvements were added to make the Reader threadsafe and support memory
mapping the CDB files.
