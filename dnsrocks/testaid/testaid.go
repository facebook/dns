/*
Copyright (c) Meta Platforms, Inc. and affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testaid

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"path"
	"testing"
	"time"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata/cdb"
	"github.com/facebookincubator/dns/dnsrocks/dnsdata/rdb"
	"github.com/facebookincubator/dns/dnsrocks/testutils"

	"github.com/stretchr/testify/assert"
)

// TestDB is a description of a test database
type TestDB struct {
	Driver string
	Path   string
}

const (
	inputFileName = "data.nets"
	cdbFileName   = "data.cdb"
)

var (
	// TestCDBBad is a test database that points to an invalid but existing CDB
	TestCDBBad = TestDB{Driver: "cdb", Path: "THIS_WILL_BE_OVERRIDDEN_BADCDB"} // Path points to something that exists, but is not in CDB format, should be overridden from Run()
	// TestCDB points to a valid CDB with data, it is pre-compiled
	TestCDB = TestDB{Driver: "cdb", Path: "THIS_WILL_BE_OVERRIDDEN_CDB"}
	// TestCDBv2 points to a valid CDB with data, it is pre-compiled
	TestCDBv2 = TestDB{Driver: "cdb", Path: "THIS_WILL_BE_OVERRIDDEN_CDBV2"}
	// TestRDB points to a temporary RDB, it is compiled on each run
	TestRDB = TestDB{Driver: "rocksdb", Path: "THIS_WILL_BE_OVERRIDDEN_RDB"}
	// TestRDBV2 points to a temporary RDB with v2 keys, it is compiled on each run
	TestRDBV2 = TestDB{Driver: "rocksdb", Path: "THIS_WILL_BE_OVERRIDDEN_RDB"}
)

// TestDBs consists of all test databases
var TestDBs []TestDB

// Run creates the test databases and runs the tests. It returns an exit code to pass to os.Exit.
func Run(m *testing.M, relativePath string) int {
	// create tempdir for RDB
	rdbDir, err := os.MkdirTemp("", "rocksdb-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(rdbDir)

	// create tempdir for RDB v2
	rdbDirV2, err := os.MkdirTemp("", "rocksdb-v2-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(rdbDirV2)

	// create tempdir for CDB
	cdbDir, err := os.MkdirTemp("", "cdb-test")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(cdbDir)

	// determine full path to inputFileName
	fullInputFileName := testutils.FixturePath(relativePath, inputFileName)
	TestCDB.Path = path.Join(cdbDir, cdbFileName)

	// temporarily suppress output to make test suite happy
	// (otherwise any output from CompileRDB() will fail the test
	err, errDB, errPath := func() (error, string, string) {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)
		// compile RDB into tempdir
		o := rdb.CompilationOptions{}
		_, err = rdb.CompileToSpecificRDBVersion(fullInputFileName, rdbDir, o)
		if err != nil {
			return err, "RDB", rdbDir
		}
		// compile RDB v2 into tempdir
		o.UseV2KeySyntax = true
		_, err = rdb.CompileToSpecificRDBVersion(fullInputFileName, rdbDirV2, o)
		if err != nil {
			return err, "RDBv2", rdbDirV2
		}
		// compile CDB into tempdir
		creatorOptions := cdb.NewDefaultCreatorOptions()
		_, err = cdb.CreateCDB(fullInputFileName, TestCDB.Path, creatorOptions)
		return err, "CDB", TestCDB.Path
	}()
	if err != nil {
		log.Fatalf("Error compiling %s (%s) to %s: %s", inputFileName, errDB, errPath, err)
	}
	TestCDBBad.Path = testutils.FixturePath(relativePath, inputFileName) // path to CDB should be relative to test executable
	TestRDB.Path = rdbDir                                                // override path to RDB
	TestRDBV2.Path = rdbDirV2
	TestDBs = []TestDB{
		TestCDB,
		TestRDB,
		TestRDBV2,
	}
	return m.Run()
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

// MkTestCert creates a temporary file PEM file and write it to disk. The file
// contains both a private key and a cert.
// returns the file name where the cert/key were saved. The caller must delete
// the file.
func MkTestCert(t *testing.T) string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.Nil(t, err)

	template := &x509.Certificate{
		Version:      1,
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "fbserver test certificate",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
	}
	pemBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	assert.Nil(t, err)

	tmpfile, err := os.CreateTemp("", "example")
	assert.Nil(t, err)

	err = pem.Encode(tmpfile, &pem.Block{Type: "CERTIFICATE", Bytes: pemBytes})
	if err != nil {
		// Fail to write cert, delete the temp file and assert.
		os.Remove(tmpfile.Name())
		assert.Nil(t, err)
	}
	err = pem.Encode(tmpfile, pemBlockForKey(privateKey))
	if err != nil {
		// Fail to write cert, delete the temp file and assert.
		os.Remove(tmpfile.Name())
		assert.Nil(t, err)
	}
	tmpfile.Close()
	return tmpfile.Name()
}
