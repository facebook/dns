package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/golang/glog"
	"github.com/repustate/go-cdb"
)

func exitOnErr(err error) {
	if err != nil {
		glog.Fatal(err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, "usage: cdbmake f [ftmp]\n")
	os.Exit(2)
}

func main() {
	var tmp *os.File
	var err error

	flag.Parse()
	args := flag.Args()
	if len(args) == 1 {
		dir, _ := path.Split(args[0])
		tmp, err = os.CreateTemp(dir, "")
		exitOnErr(err)
	} else if len(args) == 2 {
		tmp, err = os.OpenFile(args[1], os.O_RDWR|os.O_CREATE, 0644)
		exitOnErr(err)
	} else {
		usage()
	}

	fname := args[0]
	tmpname := tmp.Name()

	exitOnErr(cdb.Make(tmp, bufio.NewReader(os.Stdin)))
	exitOnErr(tmp.Sync())
	exitOnErr(tmp.Close())
	exitOnErr(os.Rename(tmpname, fname))
}
