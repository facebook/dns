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

package rdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	rocksdb "github.com/facebookincubator/dns/dnsrocks/cgo-rocksdb"
	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
)

// DefaultBatchSize is the default allocation for Batch
const DefaultBatchSize = 100000

// defaults for RocksDB options
const (
	defaultBloomFilterBits = 10
	defaultBlockCacheMB    = 8
	// compaction
	defaultWriteBufferSizeMB               = 128
	defaultTargetFileSizeBaseMB            = 64
	defaultMaxBytesForLevelBaseMB          = 512
	defaultLevel0FileNumCompactionTrigger  = 2
	defaultMinWriteBufferNumberToMerge     = 2
	defaultMaxWriteBufferNumber            = 6
	defaultCompactOnDeletionWindow         = 10000
	defaultCompactOnDeletionNumDelsTrigger = 9500
)

// Mb is megabyte
const Mb = 1 << 20

// DBI is an interface abstracting RocksDB operations. Enables mocks.
type DBI interface {
	Put(writeOptions *rocksdb.WriteOptions, key, value []byte) error
	Get(readOptions *rocksdb.ReadOptions, key []byte) ([]byte, error)
	Delete(writeOptions *rocksdb.WriteOptions, key []byte) error
	NewBatch() *rocksdb.Batch
	GetMulti(readOptions *rocksdb.ReadOptions, keys [][]byte) ([][]byte, []error)
	ExecuteBatch(batch *rocksdb.Batch, writeOptions *rocksdb.WriteOptions) error
	IngestSSTFiles(fileNames []string, useHardlinks bool) error
	Flush() error
	CreateIterator(readOptions *rocksdb.ReadOptions) *rocksdb.Iterator
	CatchWithPrimary() error
	CloseDatabase()
	GetProperty(string) string
	GetOptions() *rocksdb.Options
	CompactRangeAll()
}

// RDB is RocksDB-backed DNS database
type RDB struct {
	db           DBI
	writeMutex   *sync.Mutex
	readOptions  *rocksdb.ReadOptions
	writeOptions *rocksdb.WriteOptions
	logDir       string // RocksDB log output directory
	secondary    bool   // DB open in secondary mode

	iteratorPool *IteratorPool
}

// Context is a structure holding the state between calls to DB
type Context struct {
	cache map[string]contextCacheEntry
}

type contextCacheEntry struct {
	key  []byte
	data []byte
}

// Batch allows to batch changes (Add/Del) and then atomically execute them
type Batch struct {
	addedPairs   kvList
	deletedPairs kvList
	sorted       bool
}

// NewRDB creates an instance of RDB; path should be an existing path
// to the directory, the database will be opened or initialized
func NewRDB(path string) (*RDB, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s directory does not exist: %w", path, err)
	}

	options := rocksdb.NewOptions()
	options.EnableCreateIfMissing()
	options.SetParallelism(runtime.NumCPU())
	options.OptimizeLevelStyleCompaction(0)
	options.SetFullBloomFilter(10)    // 10 bits
	options.SetLRUCacheSize(128 * Mb) // 128 Mb

	db, err := rocksdb.OpenDatabase(path, false, false, options)
	if err != nil {
		options.FreeOptions()
		return nil, err
	}
	// We disable WAL because there is a known bug with CatchWithPrimary when it's enabled - T59258592
	writeOptions := rocksdb.NewWriteOptions(
		false, true, true, false, false,
	)
	readOptions := rocksdb.NewDefaultReadOptions()

	iteratorPool := newIteratorPool(func() *rocksdb.Iterator { return db.CreateIterator(readOptions) })
	iteratorPool.enable()

	return &RDB{
		db:           db,
		writeMutex:   &sync.Mutex{},
		readOptions:  readOptions,
		writeOptions: writeOptions,
		logDir:       path,
		iteratorPool: iteratorPool,
	}, nil
}

func getEnvVar(key string, defaultValue int) int {
	strVal := os.Getenv(key)
	if strVal == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(strVal)
	if err != nil {
		log.Printf("Failing back to default RocksDB option %s, error when getting from ENV: %v", key, err)
		return defaultValue
	}
	log.Printf("Overriding RocksDB option %s=%d through env variable", key, i)
	return i
}

// DefaultOptions returns rocksdb Options initialized with DNSROCKS default values, including potential overrides from ENV variables.
func DefaultOptions() *rocksdb.Options {
	options := rocksdb.NewOptions()
	bloom := getEnvVar("FBDNS_ROCKSDB_FULL_BLOOM_FILTER_BITS", defaultBloomFilterBits)
	options.SetFullBloomFilter(bloom)
	blockCache := getEnvVar("FBDNS_ROCKSDB_BLOCK_CACHE_MB", defaultBlockCacheMB)
	options.SetLRUCacheSize(blockCache * Mb) // N mb

	// directions for compaction fine-tuning (based on https://github.com/facebook/rocksdb/wiki/RocksDB-Tuning-Guide)
	// We can estimate level 0 size in stable state as write_buffer_size * min_write_buffer_number_to_merge * level0_file_num_compaction_trigger
	// min_write_buffer_number_to_merge = 1
	// level0_file_num_compaction_trigger = 10
	// write_buffer_size = 64Mb
	// which gives us level0 size = 1 * 10 * 64Mb = 640Mb
	//
	// with level0 size of 640Mb we need ~similar size of level1 (max_bytes_for_level_base)
	// max_bytes_for_level_base = 640Mb

	// replicate OptimizeLevelStyleCompaction (https://fburl.com/diffusion/ntv9vmga) with our overrides from env vars
	writeBufferSize := getEnvVar("FBDNS_ROCKSDB_WRITE_BUFFER_SIZE_MB", defaultWriteBufferSizeMB) * Mb
	options.SetWriteBufferSize(writeBufferSize)
	minWriteBufferNumberToMerge := getEnvVar("FBDNS_ROCKSDB_MIN_WRITE_BUFFER_NUMBER_TO_MERGE", defaultMinWriteBufferNumberToMerge)
	options.SetMinWriteBufferNumberToMerge(minWriteBufferNumberToMerge)
	maxWriteBufferNumber := getEnvVar("FBDNS_ROCKSDB_MAX_WRITE_BUFFER_NUMBER", defaultMaxWriteBufferNumber)
	options.SetMaxWriteBufferNumber(maxWriteBufferNumber)
	level0FileNumCompactionTrigger := getEnvVar("FBDNS_ROCKSDB_LEVEL0_COMPACTION_TRIGGER", defaultLevel0FileNumCompactionTrigger)
	options.SetLevel0FileNumCompactionTrigger(level0FileNumCompactionTrigger)
	targetFileSizeBase := getEnvVar("FBDNS_ROCKSDB_TARGET_FILE_SIZE_BASE_MB", defaultTargetFileSizeBaseMB) * Mb
	options.SetTargetFileSizeBase(targetFileSizeBase)
	maxBytesForLevelBase := getEnvVar("FBDNS_ROCKSDB_MAX_BYTES_FOR_LEVEL_BASE_MB", defaultMaxBytesForLevelBaseMB) * Mb
	options.SetMaxBytesForLevelBase(maxBytesForLevelBase)
	compactOnDeletionWindow := getEnvVar("FBDNS_ROCKSDB_COMPACT_ON_DELETION_WINDOW", defaultCompactOnDeletionWindow)
	compactOnDeletionNumDelsTrigger := getEnvVar("FBDNS_ROCKSDB_COMPACT_ON_DELETION_NUM_DELS_TRIGGER", defaultCompactOnDeletionNumDelsTrigger)
	options.SetCompactOnDeletion(compactOnDeletionWindow, compactOnDeletionNumDelsTrigger)
	options.EnableStatistics()

	levels := make([]rocksdb.CompressionType, rocksdb.DefaultCompactionNumLevels)
	for i := 0; i < rocksdb.DefaultCompactionNumLevels; i++ {
		// only compress levels >= 2
		if i < 2 {
			levels[i] = rocksdb.CompressionDisabled
			continue
		}
		// TODO: evaluate using zstd?
		levels[i] = rocksdb.CompressionLZ4
	}
	options.SetCompressionPerLevel(levels)
	return options
}

// NewReader creates a read-only instance of RDB; path should be an existing path
// to the directory, the database will be opened as secondary
func NewReader(path string) (*RDB, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s directory does not exist: %w", path, err)
	}

	logDir, err := os.MkdirTemp("", fmt.Sprintf("rdb-log-%d", os.Getpid()))
	if err != nil {
		return nil, err
	}

	options := DefaultOptions()
	db, err := rocksdb.OpenSecondary(path, logDir, options)
	if err != nil {
		options.FreeOptions()
		return nil, err
	}
	readOptions := rocksdb.NewDefaultReadOptions()

	iteratorPool := newIteratorPool(func() *rocksdb.Iterator { return db.CreateIterator(readOptions) })
	iteratorPool.enable()

	return &RDB{
		db:           db,
		readOptions:  readOptions,
		logDir:       logDir,
		secondary:    true,
		iteratorPool: iteratorPool,
	}, nil
}

// NewUpdater opens an existing database for update.
// It returns an instance of RDB; dbpath should be an existing path to the directory
// containing a RocksDB database.
func NewUpdater(dbpath string) (*RDB, error) {
	opt := DefaultOptions()
	// The options below were copied from NewRDB() — their effect on the update performance is not yet determined
	opt.SetParallelism(runtime.NumCPU())
	// never call opt.PrepareForBulkLoad() here, it's made to be used on empty DB only,
	// disables compaction and will simply fail on a DB that was already compacted before.
	opt.SetFullBloomFilter(fullBloomFilterBits)
	// we don't need to keep old logs or old files
	opt.SetDeleteObsoleteFilesPeriodMicros(0)
	opt.SetKeepLogFileNum(1)

	db, err := rocksdb.OpenDatabase(dbpath, false, false, opt)
	if err != nil {
		return nil, fmt.Errorf("rocksdb.OpenDatabase: %w", err)
	}

	// TODO "false, false, true, false, false" looks to cryptic - consider converting to option setters
	// We disable WAL because there is a known bug with CatchWithPrimary when it's enabled - T59258592
	wopt := rocksdb.NewWriteOptions(false, true, true, false, false)
	ropt := rocksdb.NewDefaultReadOptions()
	rdb := &RDB{
		db:           db,
		writeMutex:   &sync.Mutex{},
		readOptions:  ropt,
		writeOptions: wopt,
		logDir:       dbpath,
	}
	return rdb, nil
}

// CatchWithPrimary is the best effort catching up with primary database.
func (rdb *RDB) CatchWithPrimary() error {
	if !rdb.secondary {
		return errors.New("database is not in secondary mode")
	}

	// pooled iterators should be cleaned here
	// so DB snapshot is released
	rdb.iteratorPool.disable()

	err := rdb.db.CatchWithPrimary()
	if err != nil {
		return err
	}

	rdb.iteratorPool.enable()

	return nil
}

// Add inserts a multi-value pair of key and value
func (rdb *RDB) Add(key, value []byte) error {
	rdb.writeMutex.Lock()
	defer rdb.writeMutex.Unlock()

	oldData, err := rdb.db.Get(rdb.readOptions, key)
	if err != nil {
		return err
	}

	return rdb.db.Put(rdb.writeOptions, key, appendValues(oldData, [][]byte{value}))
}

// GetStats reports main memory stats from RocksDB.
// See https://github.com/facebook/rocksdb/wiki/Memory-usage-in-RocksDB for details.
func (rdb *RDB) GetStats() map[string]int64 {
	getProp := func(prop string) int64 {
		vs := rdb.db.GetProperty(prop)
		if vs == "" {
			log.Printf("failed fetching unknown DB property %q", prop)
			return 0
		}
		v, err := strconv.ParseInt(vs, 10, 64)
		if err != nil {
			log.Printf("failed parsing DB property %q: %v", prop, err)
		}
		return v
	}
	tableMem := getProp("rocksdb.estimate-table-readers-mem")
	memtableSize := getProp("rocksdb.cur-size-all-mem-tables")
	compactions := getProp("rocksdb.num-running-compactions")
	levelzerofiles := getProp("rocksdb.num-files-at-level0")
	stats := map[string]int64{
		"rocksdb.mem.estimate-table-readers.bytes":  tableMem,
		"rocksdb.mem.cur-size-all-mem-tables.bytes": memtableSize,
		"rocksdb.num-running-compactions":           compactions,
		"rocksdb.num-files-at-level0":               levelzerofiles,
	}

	opts := rdb.db.GetOptions()
	if opts != nil {
		cache := opts.GetCache()
		if cache != nil {
			stats["rocksdb.mem.block-cache.usage.bytes"] = int64(cache.GetUsage())
			stats["rocksdb.mem.block-cache.pinned-usage.bytes"] = int64(cache.GetPinnedUsage())
		}
	}

	if opts != nil {
		s := rdbStats(opts.GetStatisticsString())
		for k, v := range s {
			stats[k] = v
		}
	}

	return stats
}

// Del deletes the key-value pair. If the value to be deleted is the last
// value for the associated key, it will delete the key as well.
// Attempts to delete non-existing key or non-existing value will cause
// an error
func (rdb *RDB) Del(key, value []byte) error {
	rdb.writeMutex.Lock()
	defer rdb.writeMutex.Unlock()

	data, err := rdb.db.Get(rdb.readOptions, key)
	if err != nil {
		return err
	}
	if data == nil {
		// key not found
		return ErrNXKey
	}

	newData, err := delValue(data, value)
	if err != nil {
		return err
	}

	if len(newData) == 0 {
		// that was the last value for this key, delete it
		return rdb.db.Delete(rdb.writeOptions, key)
	}
	return rdb.db.Put(rdb.writeOptions, key, newData)
}

// ExecuteBatch will apply all operations from the batch. The same batch
// cannot be applied twice.
func (rdb *RDB) ExecuteBatch(batch *Batch) error {
	if batch.IsEmpty() {
		return nil
	}

	uniqueKeys := batch.getAffectedKeys()

	// lock is needed, because between getting and updating values there might be a race
	rdb.writeMutex.Lock()
	defer rdb.writeMutex.Unlock()
	dbValues, errors := rdb.db.GetMulti(rdb.readOptions, uniqueKeys)
	for _, err := range errors {
		if err != nil {
			return err // return the first error that had happened in GetMulti()
		}
	}

	dbBatch := rdb.db.NewBatch()
	defer dbBatch.Destroy()

	// update dbValues with batch contents; it assumes that uniqueKeys is sorted
	// and in the same order as dbValues
	if err := batch.integrate(uniqueKeys, &dbValues); err != nil {
		return err
	}

	for i, key := range uniqueKeys {
		val := dbValues[i]
		if len(val) == 0 {
			dbBatch.Delete(key)
		} else {
			dbBatch.Put(key, val)
		}
	}

	return rdb.db.ExecuteBatch(dbBatch, rdb.writeOptions)
}

// Close closes the database and frees up resources
func (rdb *RDB) Close() error {
	var err error
	// flush is not implemented when open as secondary
	if !rdb.secondary {
		err = rdb.db.Flush()
		if err != nil {
			log.Printf("failed flushing DB before closing it: %v", err)
		}
		log.Printf("waiting for potential compactions to finish")
		for {
			stats := rdb.GetStats()
			numComp, ok := stats["rocksdb.num-running-compactions"]
			if !ok {
				log.Printf("cannot find \"rocksdb.num-running-compactions\" key in RocksDB stats, unable to wait for potential compactions to finish")
				break
			}
			log.Printf("currently running compactions: %d", numComp)
			if numComp == 0 {
				break
			}
			time.Sleep(time.Second)
		}
	}

	if rdb.iteratorPool != nil {
		rdb.iteratorPool.disable()
	}

	if rdb.writeOptions != nil {
		rdb.writeOptions.FreeWriteOptions()
	}
	rdb.readOptions.FreeReadOptions()
	rdb.db.CloseDatabase()

	// log cleanup from tmp
	if rdb.secondary {
		err = os.RemoveAll(rdb.logDir)
		if err != nil {
			return err
		}
	}
	return err
}

// CreateBatch returns an empty batch
func (rdb *RDB) CreateBatch() *Batch {
	return &Batch{
		addedPairs:   make(kvList, 0, DefaultBatchSize),
		deletedPairs: make(kvList, 0, DefaultBatchSize),
		sorted:       true,
	}
}

// Add schedules addition of kv pair
func (batch *Batch) Add(key, value []byte) {
	batch.addedPairs = append(
		batch.addedPairs,
		keyValues{
			key:    copyBytes(key),
			values: [][]byte{copyBytes(value)},
		},
	)
	batch.sorted = false
}

// Del schedules removal of kv pair; key will be removed if this value
// is the only value stored for this key, otherwise the value will be removed
// from multivalue
func (batch *Batch) Del(key, value []byte) {
	batch.deletedPairs = append(
		batch.deletedPairs,
		keyValues{
			key:    copyBytes(key),
			values: [][]byte{copyBytes(value)},
		},
	)
	batch.sorted = false
}

// IsEmpty returns true if the batch is empty
func (batch *Batch) IsEmpty() bool {
	return len(batch.addedPairs)+len(batch.deletedPairs) == 0
}

// sort sorts addedPairs and deletedPairs if necessary
func (batch *Batch) sort() {
	if !batch.sorted {
		batch.addedPairs.Sort()
		batch.deletedPairs.Sort()
	}
	batch.sorted = true
}

// getAffectedKeys returns unique keys in the batch in the sorted order;
func (batch *Batch) getAffectedKeys() [][]byte {
	batch.sort()
	keys := make([][]byte, 0, len(batch.addedPairs)+len(batch.deletedPairs))
	aOffset, dOffset := 0, 0
	var lastKey []byte

	pushAdded := func() {
		keys = append(keys, batch.addedPairs[aOffset].key)
		lastKey = batch.addedPairs[aOffset].key
		aOffset++
	}

	pushDeleted := func() {
		keys = append(keys, batch.deletedPairs[dOffset].key)
		lastKey = batch.deletedPairs[dOffset].key
		dOffset++
	}

	for {
		// merge sorted lists
		aInRange := aOffset < len(batch.addedPairs)
		dInRange := dOffset < len(batch.deletedPairs)

		if aInRange && lastKey != nil && bytes.Equal(lastKey, batch.addedPairs[aOffset].key) {
			// skip duplicate in addedPairs
			aOffset++
			continue
		}

		if dInRange && lastKey != nil && bytes.Equal(lastKey, batch.deletedPairs[dOffset].key) {
			// skip duplicate in deletedPairs
			dOffset++
			continue
		}

		if aInRange && dInRange {
			if bytes.Compare(batch.addedPairs[aOffset].key, batch.deletedPairs[dOffset].key) < 0 {
				pushAdded()
			} else {
				pushDeleted()
			}
		} else if aInRange {
			pushAdded()
		} else if dInRange {
			pushDeleted()
		} else {
			// !aInRange && !dInRange
			break
		}
	}

	return keys
}

// integrate incorporates values in the data
func (batch *Batch) integrate(uniqueKeys [][]byte, dbValues *[][]byte) error {
	aOffset, dOffset := 0, 0
	for i, key := range uniqueKeys {
		for ; aOffset < len(batch.addedPairs) && bytes.Equal(batch.addedPairs[aOffset].key, key); aOffset++ {
			(*dbValues)[i] = appendValues((*dbValues)[i], batch.addedPairs[aOffset].values)
		}
		for ; dOffset < len(batch.deletedPairs) && bytes.Equal(batch.deletedPairs[dOffset].key, key); dOffset++ {
			var err error
			(*dbValues)[i], err = delValue((*dbValues)[i], batch.deletedPairs[dOffset].values[0])
			if err != nil {
				return err
			}
		}
	}

	if aOffset != len(batch.addedPairs) || dOffset != len(batch.deletedPairs) {
		*dbValues = nil
		return fmt.Errorf("internal error: batch integration is incorrect %d != %d || %d != %d", aOffset, len(batch.addedPairs), dOffset, len(batch.deletedPairs))
	}

	return nil
}

// NewContext creates a new structure holding state across FindNext calls
func NewContext() *Context {
	ctx := new(Context)

	ctx.cache = make(map[string]contextCacheEntry, 10)

	return ctx
}

// Reset prepares the context for a new look-up cycle.
// May be called at the start or at the end of the cycle, upon the caller's discretion
func (ctx *Context) Reset() {
}

// Find returns the first data value for the given key as a byte slice.
// Find is the same as FindStart followed by FindNext.
func (rdb *RDB) Find(key []byte, context *Context) ([]byte, error) {
	data, err := rdb.get(key, context)
	if err != nil {
		return nil, err
	}

	v, _, err := ReadNextChunk(data)

	return v, err
}

// ReadNextChunk splits next chunk from data assuming the following format
// <4 byte length><chunk>[<<4 byte length><chunk>>...]
func ReadNextChunk(data []byte) (chunk []byte, leftover []byte, err error) {
	if len(data) == 0 {
		return nil, data, io.EOF
	}
	if len(data) < 4 {
		return nil, data, io.ErrUnexpectedEOF
	}
	chunkLen := int(binary.LittleEndian.Uint32(data)) + 4
	if len(data) < chunkLen {
		return nil, data, io.ErrUnexpectedEOF
	}
	chunk = data[4:chunkLen]
	leftover = data[chunkLen:]
	return
}

// FindFirst is executed outside of any context, and returns the value of the first existing key,
// as well as key offset. If the key is not found or error happened, the key offset is -1.
//
// For instance:
// if keys A and C do not exist and key B exists, being asked for {A, B, C} FindFirst will
// return the first value of key B. The order of keys on the input DOES matter. Will return
// nil/no error if nothing was found. If key has more than one value - will return the first one anyway.
func (rdb *RDB) FindFirst(keys [][]byte) ([]byte, int, error) {
	// 4 is the length of 64-bit value in bytes. Should we declare this as a constant? That is a great debate! (TM)
	vals, errs := rdb.db.GetMulti(rdb.readOptions, keys)
	for i, val := range vals {
		if errs[i] != nil {
			return nil, -1, errs[i]
		}
		if len(val) == 0 {
			continue // skip nonexistent keys
		}
		if len(val) < 4 {
			return nil, -1, io.ErrUnexpectedEOF // malformed value header
		}
		chunkLen := int(binary.LittleEndian.Uint32(val)) + 4
		if len(val) < chunkLen {
			return nil, -1, io.ErrUnexpectedEOF // value length mismatch
		}
		return val[4:chunkLen], i, nil
	}
	return nil, -1, nil
}

// FindClosest is executed outside of context, and given the key, returns KV for either the exact key match,
// or for the largest key preceding the requested key. For instance, if key {1, 2, 3, 4} is requested, but
// such a key does not exist - it will return existing key {1, 2, 3, 3}.
func (rdb *RDB) FindClosest(key []byte, ctx *Context) ([]byte, []byte, error) {
	cachedEntry, ok := ctx.cache[string(key)]

	if ok {
		return cachedEntry.key, cachedEntry.data, nil
	}

	iterEntry := rdb.iteratorPool.get()
	iter := iterEntry.iterator
	defer func() { rdb.iteratorPool.put(iterEntry) }()

	iter.SeekForPrev(key)
	if !iter.IsValid() {
		return nil, nil, iter.GetError()
	}
	k, v := iter.Key(), iter.Value()

	ctx.update(key, k, v)

	return k, v, nil
}

// ForEach calls a function for each key match.
// The function takes a byte slice as a value and return an error.
// if error is not nil, the loop will stop.
func (rdb *RDB) ForEach(key []byte, f func(value []byte) error, ctx *Context) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()

	data, err := rdb.get(key, ctx)
	if err != nil {
		return err
	}

	for {
		var v []byte
		v, data, err = ReadNextChunk(data)

		// No more row
		if errors.Is(err, io.EOF) {
			err = nil
			break
		}
		if err != nil {
			break
		}
		if err = f(v); err != nil {
			break
		}
	}

	return err
}

// IsV2KeySyntaxUsed returns value indicating whether v2 syntax is used for DB keys
func (rdb *RDB) IsV2KeySyntaxUsed() bool {
	value, err := rdb.Find([]byte(dnsdata.FeaturesKey), NewContext())
	if err != nil {
		return false
	}

	feature := dnsdata.DecodeFeatures(value)

	return feature&dnsdata.V2KeysFeature > 0
}

func (rdb *RDB) get(key []byte, ctx *Context) (data []byte, err error) {
	cachedEntry, ok := ctx.cache[string(key)]

	if ok {
		data = cachedEntry.data
	} else {
		data, err = rdb.db.Get(rdb.readOptions, key)
		if err != nil {
			return nil, err
		}
		ctx.update(key, key, data)
	}

	return data, nil
}

func (ctx *Context) update(searchKey []byte, foundKey []byte, data []byte) {
	entry := contextCacheEntry{key: foundKey, data: data}

	ctx.cache[string(searchKey)] = entry

	if !bytes.Equal(searchKey, foundKey) {
		ctx.cache[string(foundKey)] = entry
	}
}
