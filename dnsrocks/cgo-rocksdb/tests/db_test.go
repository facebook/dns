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

package rocksdbtest

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	rocksdb "github.com/facebookincubator/dns/dnsrocks/cgo-rocksdb"
)

// primary database
var (
	db           *rocksdb.RocksDB
	readOptions  *rocksdb.ReadOptions
	writeOptions *rocksdb.WriteOptions
)

// path for primary database, if in secondary mode
var primaryDBPath string

// path for main (R/W) database
var mainDBDir string

const (
	// CLI parameter for running TestCatchup in secondary mode
	strPrimaryDB = "primary_db"
	// shared constants for catchup tests
	catchupKeyFmt     = "catchupkey%06d"
	catchupValFmt     = "catchupval%06d"
	catchupBatchSize  = 10000
	catchupTimeoutMin = 1
	catchupTerminator = '\n'
	// commands for catchup RPC
	cReady      = "READY"
	cTryCatchup = "TRY_CATCHUP"
	cFinish     = "FINISH"
	// responses for catchup RPC
	rFailNotVisible = "FAIL_NOT_VISIBLE"
	rFailPartialVis = "FAIL_PARTIAL_VISIBILITY"
	rFailMalformed  = "FAIL_MALFORMED"
	rSuccess        = "SUCCESS"
)

func init() {
	flag.StringVar(
		&primaryDBPath,
		strPrimaryDB,
		"",
		"if set, run a set of tests in secondary mode of TestCatchup",
	)
}

func runCatchup() {
	// primary_db parameter provided, so starting in secondary mode;
	// primary database is R/W and secondary is R/O
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stdout, "Started")

	secLogDir, err := os.MkdirTemp("", "rocksdb-test-secondary")
	if err != nil {
		fmt.Fprintln(os.Stdout, err.Error())
	}
	defer os.RemoveAll(secLogDir)

	fmt.Fprintln(os.Stdout, "Created tempdir", secLogDir)
	options := rocksdb.NewOptions()
	readOptions = rocksdb.NewDefaultReadOptions()
	defer readOptions.FreeReadOptions()

	fmt.Fprintln(os.Stdout, "Opening", primaryDBPath, "as secondary")
	secDB, err := rocksdb.OpenSecondary(primaryDBPath, secLogDir, options)
	if err != nil {
		options.FreeOptions()
		fmt.Fprintf(os.Stdout, "Cannot open secondary database: %s", err)
		return
	}
	defer secDB.CloseDatabase()
	fmt.Fprintln(os.Stdout, "Opened", primaryDBPath, "as secondary")

	respond := func(response string) {
		fmt.Fprintf(os.Stdout, "<%s\n", response)
	}

	respond(cReady)

ChildLoop:
	for {
		// get request
		request, err := reader.ReadString(catchupTerminator)
		if err != nil {
			fmt.Fprintln(os.Stdout, err.Error())
		}
		if len(request) < 1 || request[0] != '>' {
			fmt.Fprintf(os.Stdout, "Malformed '%s'", request)
			continue
		}

		// process request
		request = request[1 : len(request)-1]
	Switch:
		switch request {
		case cTryCatchup:
			requestKeys := make([][]byte, catchupBatchSize)
			expectedResponses := make([][]byte, catchupBatchSize)
			for i := 0; i < catchupBatchSize; i++ {
				bKey, bValue := []byte(fmt.Sprintf(catchupKeyFmt, i)), []byte(fmt.Sprintf(catchupValFmt, i))
				requestKeys[i] = bKey
				expectedResponses[i] = bValue
			}
			// make several attempts
			for i := 1; i <= 5; i++ {
				err := secDB.CatchWithPrimary()
				if err != nil {
					log.Fatalf("%v", err)
				}
				fmt.Fprintf(os.Stdout, "Trying to catch up, attempt %d...\n", i)

				responses, errors := secDB.GetMulti(readOptions, requestKeys)

				// compare response and form a cleanup batch
				nilResponses, correctResponses := 0, 0
				for i := 0; i < catchupBatchSize; i++ {
					if responses[i] == nil {
						nilResponses++
						continue
					}
					if !bytes.Equal(responses[i], expectedResponses[i]) {
						fmt.Fprintf(os.Stdout,
							"Byte mismatch for key %v: %v / %v",
							requestKeys[i], responses[i], expectedResponses[i],
						)
						respond(rFailMalformed)
						break Switch
					}
					if errors[i] != nil {
						fmt.Fprintf(os.Stdout, "Error reading key %v: %s", requestKeys[i], errors[i].Error())
						respond(rFailMalformed)
						break Switch
					}
					correctResponses++
				}

				if correctResponses == catchupBatchSize {
					respond(rSuccess)
					break Switch // inner loop
				}

				if nilResponses != 0 && correctResponses != 0 {
					respond(rFailPartialVis)
					break Switch
				}

				time.Sleep(1 * time.Second)
			}
			respond(rFailNotVisible)
		case cFinish:
			fmt.Fprintln(os.Stdout, "Finishing")
			break ChildLoop
		default:
			fmt.Fprintf(os.Stdout, "Unrecognised request: '%s'", request)
		}
	}
	fmt.Fprintln(os.Stdout, "Cleanup")

	if err = os.RemoveAll(secLogDir); err != nil {
		fmt.Fprintf(os.Stdout, "Cannot remove %s: %s", secLogDir, err.Error())
	}
	fmt.Fprintln(os.Stdout, "Done")
}

func runPrimary(t *testing.T) {
	// Primary mode: spawn a child process in secondary mode and check if the child can see our writes

	doneChannel := make(chan bool)
	go func() {
		// set timeout for the whole test
		timeout := make(chan bool)
		go func() {
			time.Sleep(catchupTimeoutMin * time.Minute)
			timeout <- true
		}()
		select {
		case <-doneChannel:
			return
		case <-timeout:
			panic("Wait timeout in TestCatchup")
		}
	}()

	if mainDBDir == "" {
		t.Error("Internal test error: mainDBDir is empty")
		return
	}

	cmd := exec.Command(
		os.Args[0],
		"-test.run", "TestCatchup",
		"-"+strPrimaryDB, mainDBDir,
	)

	childStdin, err := cmd.StdinPipe()
	if err != nil {
		t.Errorf("Error opening stdin: %s", err.Error())
	}
	defer childStdin.Close()

	childStdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Errorf("Error opening stdout: %s", err.Error())
	}
	defer childStdout.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	responses := make(chan string, 50)
	go func() {
		scanner := bufio.NewScanner(childStdout)
		for scanner.Scan() {
			txt := scanner.Text()
			fmt.Println("<<<", txt) // debug echo
			if len(txt) > 0 && txt[0] == '<' {
				// command response
				responses <- txt[1:]
			}
		}
		close(responses)
		wg.Done()
	}()

	commands := make(chan string)
	defer close(commands)
	go func() {
		for cmd := range commands {
			fmt.Printf("Sending command: %s\n", cmd)
			_, err := io.WriteString(childStdin, ">"+cmd+"\n")
			if err != nil {
				log.Fatalf("%v", err)
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		t.Errorf("Error spawning child process: %s", err.Error())
	}

ParentLoop:
	for resp := range responses {
		switch resp {
		case cReady:
			// secondary is ready, write a batch
			if err := fillValues(catchupKeyFmt, catchupValFmt, catchupBatchSize); err != nil {
				t.Error(err)
			}
			// command child to catch up
			commands <- cTryCatchup
		case rFailNotVisible:
			t.Error("Child was unable to see the changes")
			break ParentLoop
		case rFailMalformed:
			t.Error("Child saw malformed changes")
			break ParentLoop
		case rFailPartialVis:
			t.Error("Child saw only part of the changes")
			break ParentLoop
		case rSuccess:
			fmt.Println("Caught up successfully")
			break ParentLoop
		default:
			fmt.Printf("Unrecognized response: '%s'\n", resp)
		}
	}
	fmt.Println("Commanding to finish")
	commands <- cFinish

	fmt.Println("Waiting")
	if err := cmd.Wait(); err != nil {
		t.Error("Non-zero exit code from the child process")
	}
	wg.Wait()

	doneChannel <- true
	close(doneChannel)
}

func TestCatchup(t *testing.T) {
	// the primary database is opened in R/W mode, and the secondary in R/O mode;
	// secondary process is spawned and guided by the primary
	if primaryDBPath != "" {
		runCatchup()
	} else {
		runPrimary(t)
	}
}

func TestMain(m *testing.M) {
	var err error
	mainDBDir, err = os.MkdirTemp("", "rocksdb-test")

	if err != nil {
		log.Fatal(err)
	}
	// defer will fire up in case of failure, otherwise there is yet another
	// RemoveAll call before os.Exit()
	defer os.RemoveAll(mainDBDir)

	options := rocksdb.NewOptions()
	options.EnableCreateIfMissing()
	options.SetParallelism(runtime.NumCPU())
	options.OptimizeLevelStyleCompaction(0)
	options.SetFullBloomFilter(10)
	options.SetLRUCacheSize(32 << 20) // 32 Mb

	writeOptions = rocksdb.NewDefaultWriteOptions()
	defer writeOptions.FreeWriteOptions()
	readOptions = rocksdb.NewDefaultReadOptions()
	defer readOptions.FreeReadOptions()

	db, err = rocksdb.OpenDatabase(mainDBDir, false, false, options)
	if err != nil {
		options.FreeOptions()
		log.Fatal("Cannot create database", err)
	}
	defer db.CloseDatabase()

	exitCode := m.Run()

	if err = os.RemoveAll(mainDBDir); err != nil {
		log.Fatal("Cannot remove", mainDBDir, err.Error())
	}
	os.Exit(exitCode)
}

// TestStrValue tests PutStr, GetStr, DeleteStr for a single string value
func TestStrValue(t *testing.T) {
	sKey, sValue := "test", "response"

	// PutStr
	if err := db.PutStr(writeOptions, sKey, sValue); err != nil {
		t.Errorf("Error writing string: %s", err.Error())
	}

	// GetStr
	if res, err := db.GetStr(readOptions, sKey); err != nil {
		t.Errorf("Error reading string: %s", err.Error())
	} else if res != sValue {
		t.Errorf("String mismatch: %s / %s", res, sValue)
	}

	// DeleteStr
	if err := db.DeleteStr(writeOptions, sKey); err != nil {
		t.Errorf("Error deleting string: %s", err.Error())
	}

	// GetStr to validate it is gone
	if res, err := db.GetStr(readOptions, sKey); err != nil {
		t.Errorf("Error reading deleted string: %s", err.Error())
	} else if res != "" {
		// nonexistent string value is ""
		t.Errorf("String mismatch: %s / %s", res, sValue)
	}
}

// TestByteValue tests Put, Get, Delete for a single byte[] value
func TestByteValue(t *testing.T) {
	bKey, bValue := []byte{0, 1, 2, 3, 4}, []byte{5, 6, 7, 8}

	// Put
	if err := db.Put(writeOptions, bKey, bValue); err != nil {
		t.Errorf("Error writing bytes: %s", err.Error())
	}

	// Get
	if res, err := db.Get(readOptions, bKey); err != nil {
		t.Errorf("Error reading string: %s", err.Error())
	} else if !bytes.Equal(res, bValue) {
		t.Errorf("Byte mismatch: %v / %v", res, bValue)
	}

	// Delete
	if err := db.Delete(writeOptions, bKey); err != nil {
		t.Errorf("Error deleting bytes: %s", err.Error())
	}

	// Get to validate it is gone
	if res, err := db.Get(readOptions, bKey); err != nil {
		t.Errorf("Error reading deleted bytes: %s", err.Error())
	} else if res != nil {
		// nonexistent bytes value is nil
		t.Errorf("Bytes mismatch: %v / %v", res, bValue)
	}
}

// TestBatch tests creating, writing and deleting a batch
func TestBatch(t *testing.T) {
	batch := db.NewBatch()
	defer batch.Destroy()
	const batchSize = 10000
	const keyFmt = "key%06d"
	const valFmt = "val%06d"
	for i := 0; i < batchSize; i++ {
		batch.Put([]byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i)))
	}

	// validate GetCount()
	if itemCount := batch.GetCount(); itemCount != batchSize {
		t.Errorf("Batch size mismatch: %d / %d", itemCount, batchSize)
	}

	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing write batch: %s", err.Error())
	}

	// validate Clear(); GetCount() is already validated
	batch.Clear()
	if itemCount := batch.GetCount(); itemCount != 0 {
		t.Errorf("The batch still has %d elements", itemCount)
	}

	// validate writes by reading, and form a Delete batch
	for i := 0; i < batchSize; i++ {
		sKey, sValue := fmt.Sprintf(keyFmt, i), fmt.Sprintf(valFmt, i)
		// read as strings
		if resStr, err := db.GetStr(readOptions, sKey); err != nil {
			t.Errorf("Error reading string: %s", err.Error())
		} else if resStr != sValue {
			t.Errorf("String mismatch: %s / %s", resStr, sValue)
		}

		bKey, bValue := []byte(sKey), []byte(sValue)
		// read as bytes
		if resBytes, err := db.Get(readOptions, bKey); err != nil {
			t.Errorf("Error reading bytes: %s", err.Error())
		} else if !bytes.Equal(resBytes, bValue) {
			t.Errorf("Byte mismatch: %v / %v", resBytes, bValue)
		}

		// prepare cleanup
		batch.Delete(bKey)
	}

	// validate that the batch contains the expected number of Delete()
	if itemCount := batch.GetCount(); itemCount != batchSize {
		t.Errorf("Batch size mismatch: %d / %d", itemCount, batchSize)
	}

	// execute cleanup
	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing delete batch: %s", err.Error())
	}

	// check that the key is gone
	if res, err := db.GetStr(readOptions, fmt.Sprintf(keyFmt, 0)); err != nil {
		t.Errorf("Error reading string: %s", err.Error())
	} else if res != "" {
		t.Error("The key was not deleted")
	}
}

// fillValues adds count of kv pairs matching provided format
func fillValues(keyFmt, valFmt string, count int) error {
	batch := db.NewBatch()
	defer batch.Destroy()
	for i := 0; i < count; i++ {
		bKey, bValue := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
		batch.Put(bKey, bValue)
	}

	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		return fmt.Errorf("Error executing write batch in fillValues: %s", err.Error())
	}
	return nil
}

// TestMulti tests writing (with batches) and reading (with GetMulti) multiple values.
func TestMulti(t *testing.T) {
	const batchSize = 10000
	const keyFmt = "multi_key%06d"
	const valFmt = "multi_val%06d"
	if err := fillValues(keyFmt, valFmt, batchSize); err != nil {
		t.Error(err)
	}

	requestKeys := make([][]byte, batchSize+1)
	expectedResponses := make([][]byte, batchSize+1)
	for i := 0; i < batchSize; i++ {
		bKey, bValue := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
		requestKeys[i] = bKey
		expectedResponses[i] = bValue
	}
	requestKeys[batchSize] = []byte("NON_EXISTENT_KEY")
	expectedResponses[batchSize] = nil
	responses, errors := db.GetMulti(readOptions, requestKeys)

	// compare response and form a cleanup batch
	batch := db.NewBatch()
	for i := 0; i < batchSize; i++ {
		if !bytes.Equal(responses[i], expectedResponses[i]) {
			t.Errorf(
				"Byte mismatch for key %v: %v / %v",
				requestKeys[i], responses[i], expectedResponses[i],
			)
		}
		if errors[i] != nil {
			t.Errorf("Error reading key %v: %s", requestKeys[i], errors[i].Error())
		}
		batch.Delete(requestKeys[i])
	}

	// validate that the batch contains the expected number of Delete()
	if itemCount := batch.GetCount(); itemCount != batchSize {
		t.Errorf("Batch size mismatch: %d / %d", itemCount, batchSize)
	}

	// execute cleanup
	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing delete batch: %s", err.Error())
	}
}

// TestIterator tests writing (with batches) and reading (with Iterator) multiple values.
func TestIterator(t *testing.T) {
	const batchSize = 10000
	const keyFmt = "iterator_key%06d"
	const valFmt = "iterator_val%06d"
	if err := fillValues(keyFmt, valFmt, batchSize); err != nil {
		t.Error(err)
	}

	t.Run("TestIterator_Forward", func(t *testing.T) {
		t.Parallel()

		iter := db.CreateIterator(readOptions)
		defer iter.FreeIterator()

		for i := 0; i < batchSize; i++ {
			bKey, bVal := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
			switch i {
			case 0:
				iter.Seek(bKey)
			case 1:
				iter.SeekForPrev(bKey)
			}
			if !iter.IsValid() {
				t.Errorf("Invalid iterator on key %s, error %s", bKey, iter.GetError())
			}
			rKey, rVal := iter.Key(), iter.Value()
			if !bytes.Equal(bKey, rKey) {
				t.Errorf("Key mismatch: expected %s, got %s", bKey, rKey)
			}
			if !bytes.Equal(bVal, rVal) {
				t.Errorf("Value mismatch: expected %s, got %s", bVal, rVal)
			}
			iter.Next()
		}
		if err := iter.GetError(); err != nil {
			t.Error(err)
		}
	})

	t.Run("TestIterator_Back", func(t *testing.T) {
		t.Parallel()

		iter := db.CreateIterator(readOptions)
		defer iter.FreeIterator()

		for i := batchSize - 1; i >= 0; i-- {
			bKey, bVal := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
			if i == batchSize-1 {
				iter.Seek(bKey)
			}
			if !iter.IsValid() {
				t.Errorf("Invalid iterator on key %s, error %s", bKey, iter.GetError())
			}
			rKey, rVal := iter.Key(), iter.Value()
			if !bytes.Equal(bKey, rKey) {
				t.Errorf("Key mismatch: expected %s, got %s", bKey, rKey)
			}
			if !bytes.Equal(bVal, rVal) {
				t.Errorf("Value mismatch: expected %s, got %s", bVal, rVal)
			}
			iter.Prev()
		}
		if err := iter.GetError(); err != nil {
			t.Error(err)
		}
	})

	t.Run("TestIterator_ImpreciseSeek", func(t *testing.T) {
		t.Parallel()

		iter := db.CreateIterator(readOptions)
		defer iter.FreeIterator()

		checkPair := func(val int) {
			bKey, bVal := []byte(fmt.Sprintf(keyFmt, val)), []byte(fmt.Sprintf(valFmt, val))
			if !iter.IsValid() {
				t.Errorf("Invalid iterator on key %s, error %s", bKey, iter.GetError())
			}
			rKey, rVal := iter.Key(), iter.Value()
			if !bytes.Equal(bKey, rKey) {
				t.Errorf("Key mismatch: expected %s, got %s", bKey, rKey)
			}
			if !bytes.Equal(bVal, rVal) {
				t.Errorf("Value mismatch: expected %s, got %s", bVal, rVal)
			}
		}

		// NOTE: seeking to nonexistent key "iterator_key0012340"; expected to position on the previous existing key "iterator_key001235"
		iter.Seek([]byte(fmt.Sprintf(keyFmt+"0", 1234)))

		checkPair(1235)
		iter.Prev()
		checkPair(1234)
	})
}

// TestSnapshot tests isolation between a snapshot and latest view
func TestSnapshots(t *testing.T) {
	batch := db.NewBatch()
	defer batch.Destroy()
	const batchSize = 10000
	const keyFmt = "snapkey%06d"
	const valFmtBefore = "snapval%06d"
	const valFmtAfter = "latestval%06d"
	// rogue value will show up in latest, but should be invisible in snapshot
	rogueKey, rogueValue := []byte("rogue_key"), []byte("rogue_val")
	requestKeys := make([][]byte, batchSize+1)
	expectedSnapshotResponses := make([][]byte, batchSize+1) // +1 for rogue
	expectedLatestResponses := make([][]byte, batchSize+1)

	// insert test data
	for i := 0; i < batchSize; i++ {
		bKey, bValue := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmtBefore, i))
		batch.Put(bKey, bValue)
		requestKeys[i] = bKey
		expectedSnapshotResponses[i] = bValue
	}
	requestKeys[len(requestKeys)-1] = rogueKey
	expectedSnapshotResponses[len(expectedSnapshotResponses)-1] = nil // rogue key does not exist in snapshot

	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing write batch in TestSnapshots: %s", err.Error())
	}

	// make a snapshot
	snapshot := rocksdb.NewSnapshot(db)
	defer snapshot.FreeSnapshot()
	snapshotReadOptions := rocksdb.NewReadOptions(true, true)
	defer snapshotReadOptions.FreeReadOptions()
	snapshotReadOptions.SetSnapshot(snapshot)

	// check if the snapshot matches the expectation
	invariant := func(options *rocksdb.ReadOptions, expectedKeys [][]byte, expectedValues [][]byte, testDescription string) {
		responses, errors := db.GetMulti(options, expectedKeys)
		for i := 0; i < len(expectedKeys); i++ {
			if !bytes.Equal(responses[i], expectedValues[i]) {
				t.Errorf(
					"Byte mismatch during %s test for key '%s': '%s' received, whereas '%s' expected",
					testDescription, string(expectedKeys[i]), string(responses[i]), string(expectedValues[i]),
				)
			}
			if errors[i] != nil {
				t.Errorf("Error reading key %s: %s during %s", string(expectedKeys[i]), errors[i].Error(), testDescription)
			}
		}
	}

	// run the check for original dataset; at this point latest data is equivalent to data in snapshot
	invariant(snapshotReadOptions, requestKeys, expectedSnapshotResponses, "snapshot initial")
	invariant(readOptions, requestKeys, expectedSnapshotResponses, "latest initial")

	// update the dataset, instead of 'snapval' it will contain 'latestval'
	batch.Clear()
	for i := 0; i < batchSize; i++ {
		bKey, bValue := requestKeys[i], []byte(fmt.Sprintf(valFmtAfter, i))
		batch.Put(bKey, bValue)
		expectedLatestResponses[i] = bValue
	}
	// add rogue key
	batch.Put(rogueKey, rogueValue)
	expectedLatestResponses[len(expectedLatestResponses)-1] = rogueValue
	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing update batch in TestSnapshots: %s", err.Error())
	}

	// run the check after data is updated, the snapshot should keep old values
	invariant(snapshotReadOptions, requestKeys, expectedSnapshotResponses, "snapshot updated")
	invariant(readOptions, requestKeys, expectedLatestResponses, "latest updated") // latest data should be visible without snapshot

	// delete test data
	batch.Clear()
	for i := 0; i < batchSize; i++ {
		batch.Delete(requestKeys[i])
		expectedLatestResponses[i] = nil
	}
	batch.Delete(rogueKey)
	expectedLatestResponses[len(expectedLatestResponses)-1] = nil
	if err := db.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing delete batch in TestSnapshots: %s", err.Error())
	}

	// run the check after data is deleted, the snapshot should keep old values
	invariant(snapshotReadOptions, requestKeys, expectedSnapshotResponses, "snapshot final")
	invariant(readOptions, requestKeys, expectedLatestResponses, "latest final") // and the latest is updated

	// undefine the snapshot; it should become equivalent to latest at this point
	snapshotReadOptions.UnsetSnapshot()
	invariant(snapshotReadOptions, requestKeys, expectedLatestResponses, "snapshot undefined")
}

func TestBackupRestore(t *testing.T) {
	const batchSize = 10000
	const keyFmt = "key%06d"
	const valFmt = "val%06d"
	const numBackup = 3

	dbSourceDir, err := os.MkdirTemp("", "rocksdb-test-src")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dbSourceDir)

	dbBackupDir, err := os.MkdirTemp("", "rocksdb-test-backup")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dbBackupDir)

	dbDestDir, err := os.MkdirTemp("", "rocksdb-test-dst")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dbDestDir)

	// create and populate source
	options := rocksdb.NewOptions()
	options.EnableCreateIfMissing()
	writeOptions = rocksdb.NewDefaultWriteOptions()
	defer writeOptions.FreeWriteOptions()

	dbSrc, err := rocksdb.OpenDatabase(dbSourceDir, false, false, options)
	if err != nil {
		options.FreeOptions()
		log.Fatal("Cannot create database", err)
	}
	defer dbSrc.CloseDatabase()

	// add values
	batch := dbSrc.NewBatch()
	defer batch.Destroy()
	for i := 0; i < batchSize; i++ {
		batch.Put([]byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i)))
	}

	// validate GetCount()
	if itemCount := batch.GetCount(); itemCount != batchSize {
		t.Errorf("Batch size mismatch: %d / %d", itemCount, batchSize)
	}

	if err := dbSrc.ExecuteBatch(batch, writeOptions); err != nil {
		t.Errorf("Error executing write batch: %s", err.Error())
	}

	backupEngine, err := rocksdb.NewBackupEngine(dbBackupDir)
	if err != nil {
		t.Errorf("Error creating backup engine: %s", err.Error())
	}

	// backup several times
	for i := 0; i < numBackup; i++ {
		bKey, bValue := []byte(fmt.Sprintf(keyFmt, batchSize+i)), []byte(fmt.Sprintf(valFmt, batchSize+i))
		if err := dbSrc.Put(writeOptions, bKey, bValue); err != nil {
			t.Errorf("Error writing bytes: %s", err.Error())
		}
		err = backupEngine.BackupDatabase(dbSrc, true)
		if err != nil {
			t.Errorf("Error backing up database %s: %s", dbSourceDir, err.Error())
		}
	}

	// purge all backups but the last one
	err = backupEngine.PurgeOldBackups(1)
	if err != nil {
		t.Errorf("Error purging old backups: %v", err)
	}

	err = backupEngine.RestoreFromLastBackup(dbDestDir, false)
	if err != nil {
		t.Errorf("Error restoring database: %s", err.Error())
	}

	// open restored database
	dstOptions := rocksdb.NewOptions()
	dstOptions.EnableCreateIfMissing()

	dbDst, err := rocksdb.OpenDatabase(dbDestDir, false, false, dstOptions)
	if err != nil {
		dstOptions.FreeOptions()
		log.Fatal("Cannot create database", err)
	}

	// the last written key should be visible - unless a wrong database was purged
	bKey, bValue := []byte(fmt.Sprintf(keyFmt, batchSize+numBackup-1)), []byte(fmt.Sprintf(valFmt, batchSize+numBackup-1))
	if res, err := dbDst.Get(readOptions, bKey); err != nil {
		t.Errorf("Error reading string: %s", err.Error())
	} else if !bytes.Equal(res, bValue) {
		t.Errorf("Byte mismatch: %v / %v", res, bValue)
	}

	// dir cleanup
	for _, dir := range []string{dbSourceDir, dbBackupDir, dbDestDir} {
		if err = os.RemoveAll(dir); err != nil {
			log.Fatal("Cannot remove", dir, err.Error())
		}
	}
}
