//go:build darwin
// +build darwin

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

package rocksdb

/*
// @fb-only: #include "rocksdb/src/include/rocksdb/c.h"
#cgo pkg-config: "rocksdb"
#include "rocksdb/c.h" // @oss-only

// RocksDB-compatible boolean values
const unsigned char BOOL_CHAR_FALSE = 0;
const unsigned char BOOL_CHAR_TRUE = 1;
const int BOOL_INT_FALSE = 0;
const int BOOL_INT_TRUE = 1;
*/
import (
	"C"
)

// BoolToChar is a helper to convert boolean value to C.uchar
func BoolToChar(b bool) C.uchar {
	if b {
		return C.BOOL_CHAR_TRUE
	}
	return C.BOOL_CHAR_FALSE
}

// DefaultCompactionMemtableMemoryBudget is the default for compaction
// memory usage (src/include/rocksdb/options.h).
const DefaultCompactionMemtableMemoryBudget uint64 = 512 * 1024 * 1024

// DefaultCompactionNumLevels is the default number of levels for level-style compaction
const DefaultCompactionNumLevels int = 7

// CompressionType specifies the compression we use.
type CompressionType uint

// Compression types.
const (
	CompressionDisabled = CompressionType(C.rocksdb_no_compression)
	CompressionSnappy   = CompressionType(C.rocksdb_snappy_compression)
	CompressionZLib     = CompressionType(C.rocksdb_zlib_compression)
	CompressionBz2      = CompressionType(C.rocksdb_bz2_compression)
	CompressionLZ4      = CompressionType(C.rocksdb_lz4_compression)
	CompressionLZ4HC    = CompressionType(C.rocksdb_lz4hc_compression)
	CompressionXpress   = CompressionType(C.rocksdb_xpress_compression)
	CompressionZSTD     = CompressionType(C.rocksdb_zstd_compression)
)

// CompactionStyle specifies the compaction style.
type CompactionStyle uint

// Compaction styles.
const (
	CompactionStyleLevel     = CompactionStyle(C.rocksdb_level_compaction)
	CompactionStyleUniversal = CompactionStyle(C.rocksdb_universal_compaction)
	CompactionStyleFIFO      = CompactionStyle(C.rocksdb_fifo_compaction)
)

// Options represents DB-level connection options
type Options struct {
	cOptions          *C.rocksdb_options_t
	blockBasedOptions *BlockBasedOptions
}

// NewOptions creates and returns default Options structure.
// (old) Parameters:
//  - createIfMissing: create the DB if it does not exist
//    (otherwise - throw error if exists)
//  - errorIfExits: throw error if the DB already exists
func NewOptions() *Options {
	cOptions := C.rocksdb_options_create()
	return &Options{
		cOptions: cOptions,
	}
}

// EnableCreateIfMissing flags that the database should be created if it doesn't exist.
// The default behaviour is to fail in that situation.
func (options *Options) EnableCreateIfMissing() {
	// default is False
	C.rocksdb_options_set_create_if_missing(options.cOptions, C.BOOL_CHAR_TRUE)
}

// SetDeleteObsoleteFilesPeriodMicros sets the periodicity when obsolete files get deleted. The default
// value is 6 hours. The files that get out of scope by compaction
// process will still get automatically delete on every compaction,
// regardless of this setting
func (options *Options) SetDeleteObsoleteFilesPeriodMicros(micros uint) {
	C.rocksdb_options_set_delete_obsolete_files_period_micros(options.cOptions, C.ulonglong(micros))
}

// SetKeepLogFileNum sets maximal info log files to be kept.
func (options *Options) SetKeepLogFileNum(num uint) {
	C.rocksdb_options_set_keep_log_file_num(options.cOptions, C.ulong(num))
}

// EnableErrorIfExists flags that an existing database is not expected and opening it should not succeed.
// By default, opening an existing database is allowed.
func (options *Options) EnableErrorIfExists() {
	// default is False
	C.rocksdb_options_set_error_if_exists(options.cOptions, C.BOOL_CHAR_TRUE)
}

// SetParallelism sets the number of background threads for flush and compaction
// processes, recommended to set to the number of cores. You almost definitely want
// to call this function if your system is bottlenecked by RocksDB
func (options *Options) SetParallelism(parallelism int) {
	C.rocksdb_options_increase_parallelism(options.cOptions, C.int(parallelism))
}

// SetMaxOpenFiles sets the maximum number of open files.
// -1 - unlimited
func (options *Options) SetMaxOpenFiles(numFiles int) {
	C.rocksdb_options_set_max_open_files(options.cOptions, C.int(numFiles))
}

// OptimizeLevelStyleCompaction - read more about Level Style compaction
// at https://github.com/facebook/rocksdb/wiki/Leveled-Compaction ,
// or about compactions in general https://github.com/facebook/rocksdb/wiki/Compaction
// memtableMemoryBudget = 0 will set the default value
func (options *Options) OptimizeLevelStyleCompaction(memtableMemoryBudget uint64) {
	if memtableMemoryBudget == 0 {
		memtableMemoryBudget = DefaultCompactionMemtableMemoryBudget
	}
	C.rocksdb_options_optimize_level_style_compaction(options.cOptions, C.uint64_t(memtableMemoryBudget))
}

// OptimizeUniversalStyleCompaction - read more at
// https://github.com/facebook/rocksdb/wiki/Universal-Compaction
// memtableMemoryBudget = 0 will set the default value
func (options *Options) OptimizeUniversalStyleCompaction(memtableMemoryBudget uint64) {
	if memtableMemoryBudget == 0 {
		memtableMemoryBudget = DefaultCompactionMemtableMemoryBudget
	}
	C.rocksdb_options_optimize_universal_style_compaction(options.cOptions, C.uint64_t(memtableMemoryBudget))
}

// PrepareForBulkLoad switches to the bulk load mode (no compaction, etc)
func (options *Options) PrepareForBulkLoad() {
	C.rocksdb_options_prepare_for_bulk_load(options.cOptions)
}

// SetFullBloomFilter activates full Bloom filter
func (options *Options) SetFullBloomFilter(fullBloomBits int) {
	if options.blockBasedOptions == nil {
		options.blockBasedOptions = NewBlockBasedOptions()
	}
	options.blockBasedOptions.SetFullBloomFilter(fullBloomBits)
	C.rocksdb_options_set_block_based_table_factory(
		options.cOptions,
		options.blockBasedOptions.cBlockBasedOptions,
	)
}

// some compaction options

// SetNumLevels sets number of compaction levels when level-based compaction is used.
func (options *Options) SetNumLevels(numLevels int) {
	C.rocksdb_options_set_num_levels(
		options.cOptions,
		C.int(numLevels),
	)
}

// SetCompactionStyle sets compaction style.
func (options *Options) SetCompactionStyle(style CompactionStyle) {
	C.rocksdb_options_set_compaction_style(
		options.cOptions,
		C.int(style),
	)
}

// SetCompression sets global compression type to be used.
func (options *Options) SetCompression(compression CompressionType) {
	C.rocksdb_options_set_compression(
		options.cOptions,
		C.int(compression),
	)
}

// SetBottommostCompression sets compression type to be used on bottom-most level on compaction.
func (options *Options) SetBottommostCompression(compression CompressionType) {
	C.rocksdb_options_set_bottommost_compression(
		options.cOptions,
		C.int(compression),
	)
}

// SetCompressionPerLevel sets different compression algorithm per level.
// When this option is used, number of compaction levels is set to len(levelValues),
// and specified compression type is set for each level, i.e. levelValues[0] compression is applied to level0/
func (options *Options) SetCompressionPerLevel(levelValues []CompressionType) {
	cValues := make([]C.int, len(levelValues))
	for i, v := range levelValues {
		cValues[i] = C.int(v)
	}

	C.rocksdb_options_set_compression_per_level(
		options.cOptions,
		&cValues[0],
		C.size_t(len(levelValues)),
	)
}

// SetMaxWriteBufferNumber sets maximum number of memtables, both active and immutable.
// If the active memtable fills up and the total number of memtables is larger than max_write_buffer_number we stall further writes.
// This may happen if the flush process is slower than the write rate.
func (options *Options) SetMaxWriteBufferNumber(num int) {
	C.rocksdb_options_set_max_write_buffer_number(
		options.cOptions,
		C.int(num),
	)
}

// SetWriteBufferSize sets the write_buffer_size option which
// sets the size of a single memtable.
// Once memtable exceeds this size, it is marked immutable and a new one is created.
func (options *Options) SetWriteBufferSize(size int) {
	C.rocksdb_options_set_write_buffer_size(
		options.cOptions,
		C.ulong(size),
	)
}

// SetTargetFileSizeBase sets target_file_size_base option.
// Files in level 1 will have target_file_size_base bytes.
// Each next level's file size will be target_file_size_multiplier bigger than previous one.
// Increasing target_file_size_base will reduce total number of database files, which is generally a good thing.
// It's recommended setting target_file_size_base to be max_bytes_for_level_base / 10, so that there are 10 files in level 1.
func (options *Options) SetTargetFileSizeBase(size int) {
	C.rocksdb_options_set_target_file_size_base(
		options.cOptions,
		C.ulonglong(size),
	)
}

// SetMaxBytesForLevelBase sets max_bytes_for_level_base option.
// max_bytes_for_level_base is total size of level 1.
// It's recommended that this be around the size of level 0.
func (options *Options) SetMaxBytesForLevelBase(size int) {
	C.rocksdb_options_set_max_bytes_for_level_base(
		options.cOptions,
		C.ulonglong(size),
	)
}

// SetLevel0FileNumCompactionTrigger sets level0_file_num_compaction_trigger option.
// Once level 0 reaches this number of files, L0->L1 compaction is triggered.
// One ca estimate level 0 size in stable state as write_buffer_size * min_write_buffer_number_to_merge * level0_file_num_compaction_trigger
func (options *Options) SetLevel0FileNumCompactionTrigger(num int) {
	C.rocksdb_options_set_level0_file_num_compaction_trigger(
		options.cOptions,
		C.int(num),
	)
}

// SetMinWriteBufferNumberToMerge sets min_write_buffer_number_to_merge option.
// min_write_buffer_number_to_merge is the minimum number of memtables to be merged before flushing to storage.
// For example, if this option is set to 2 (default is 1), immutable memtables are only flushed when there are two of them - a single immutable memtable will never be flushed.
// If multiple memtables are merged together, less data may be written to storage since two updates are merged to a single key.
// However, every Get() must traverse all immutable memtables linearly to check if the key is there.
// Setting this option too high may hurt read performance.
func (options *Options) SetMinWriteBufferNumberToMerge(num int) {
	C.rocksdb_options_set_min_write_buffer_number_to_merge(
		options.cOptions,
		C.int(num),
	)
}

// end of compaction options

// SetLRUCacheSize activates LRU Cache
func (options *Options) SetLRUCacheSize(capacity int) {
	if options.blockBasedOptions == nil {
		options.blockBasedOptions = NewBlockBasedOptions()
	}
	options.blockBasedOptions.SetLRUCache(capacity)
	C.rocksdb_options_set_block_based_table_factory(
		options.cOptions,
		options.blockBasedOptions.cBlockBasedOptions,
	)
}

// GetCache provides access to block cache
func (options *Options) GetCache() *LRUCache {
	if options.blockBasedOptions == nil {
		return nil
	}
	return options.blockBasedOptions.lruCache
}

// FreeOptions frees up the memory previously allocated by NewOptions
func (options *Options) FreeOptions() {
	if options.blockBasedOptions != nil {
		options.blockBasedOptions.FreeBlockBasedOptions()
	}
	C.rocksdb_options_destroy(options.cOptions)
}

// WriteOptions is a set of options for write operations
type WriteOptions struct {
	cWriteOptions *C.rocksdb_writeoptions_t
}

// NewDefaultWriteOptions creates WriteOptions object with default properties
func NewDefaultWriteOptions() *WriteOptions {
	return &WriteOptions{
		cWriteOptions: C.rocksdb_writeoptions_create(),
	}
}

// NewWriteOptions creates WriteOptions object
// Parameters:
// - syncOnWrite forces OS buffer flush on each write (slow!)
// - disableWAL disables write-ahead-log (possible data loss in case of a crash)
// - ignoreMissingColumnFamilies do not fail, and just ignore writes to non-existing
// column families (if the WriteBatch contains multiple writes - some of them will succeed)
// - failOnSlowDown forces failure with Status::Incomplete() if a write request will cause
// a wait or sleep
// - lowPriority will cause an error with Status::Incomplete() if compaction is ongoing during write request;
// this will guarantee minimum impact on high-pri writes
func NewWriteOptions(
	syncOnWrite, disableWAL, ignoreMissingColumnFamilies, failOnSlowDown, lowPriority bool,
) *WriteOptions {
	writeOptions := NewDefaultWriteOptions()
	cWriteOptions := writeOptions.cWriteOptions
	if syncOnWrite {
		// default: False
		C.rocksdb_writeoptions_set_sync(cWriteOptions, C.BOOL_CHAR_TRUE)
	}
	if disableWAL {
		// default: 0 (False)
		C.rocksdb_writeoptions_disable_WAL(cWriteOptions, C.BOOL_INT_TRUE)
	}
	if ignoreMissingColumnFamilies {
		// default: False
		C.rocksdb_writeoptions_set_ignore_missing_column_families(cWriteOptions, C.BOOL_CHAR_TRUE)
	}
	if failOnSlowDown {
		// default: False
		C.rocksdb_writeoptions_set_no_slowdown(cWriteOptions, C.BOOL_CHAR_TRUE)
	}
	if lowPriority {
		// default: False
		C.rocksdb_writeoptions_set_low_pri(cWriteOptions, C.BOOL_CHAR_TRUE)
	}
	return writeOptions
}

// FreeWriteOptions frees up the memory previously allocated by NewWriteOptions
func (writeOptions *WriteOptions) FreeWriteOptions() {
	C.rocksdb_writeoptions_destroy(writeOptions.cWriteOptions)
}

// Snapshot represents a snapshot-in-time; useful for having a consistent view
// between multiple read operations
type Snapshot struct {
	db        *RocksDB
	cSnapshot *C.rocksdb_snapshot_t
}

// NewSnapshot creates a napshot for the database
func NewSnapshot(db *RocksDB) *Snapshot {
	return &Snapshot{
		db:        db,
		cSnapshot: C.rocksdb_create_snapshot(db.cDB),
	}
}

// FreeSnapshot releases the snapshot, should be explicitly called to free up the memory
func (snapshot *Snapshot) FreeSnapshot() {
	C.rocksdb_release_snapshot(snapshot.db.cDB, snapshot.cSnapshot)
}

// ReadOptions is a set of options for read operations
type ReadOptions struct {
	cReadOptions *C.rocksdb_readoptions_t
}

// NewDefaultReadOptions creates ReadOptions object with default properties
func NewDefaultReadOptions() *ReadOptions {
	return &ReadOptions{
		cReadOptions: C.rocksdb_readoptions_create(),
	}
}

// NewReadOptions creates ReadOptions object
// Parameters:
//  - verifyChecksum: all data read from underlying storage will be
//  verified against corresponding checksums
//  - fillCache: Should the "data block"/"index block"" read for this
// iteration be placed in block cache?
func NewReadOptions(verifyChecksum, fillCache bool) *ReadOptions {
	readOptions := NewDefaultReadOptions()
	cReadOptions := readOptions.cReadOptions
	if !verifyChecksum {
		// default: True
		C.rocksdb_readoptions_set_verify_checksums(cReadOptions, C.BOOL_CHAR_FALSE)
	}
	if !fillCache {
		// default: True
		C.rocksdb_readoptions_set_fill_cache(cReadOptions, C.BOOL_CHAR_FALSE)
	}
	return readOptions
}

// SetSnapshot forces using previously made 'snapshot' for read operations
func (readOptions *ReadOptions) SetSnapshot(snapshot *Snapshot) {
	C.rocksdb_readoptions_set_snapshot(readOptions.cReadOptions, snapshot.cSnapshot)
}

// UnsetSnapshot turns off using the snapshot. The caller should dispose previously
// allocated Snapshot object
func (readOptions *ReadOptions) UnsetSnapshot() {
	C.rocksdb_readoptions_set_snapshot(readOptions.cReadOptions, nil)
}

// FreeReadOptions frees up the memory previously allocated by NewReadOptions
func (readOptions *ReadOptions) FreeReadOptions() {
	C.rocksdb_readoptions_destroy(readOptions.cReadOptions)
}
