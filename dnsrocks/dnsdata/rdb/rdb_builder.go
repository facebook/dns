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
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"time"

	rocksdb "github.com/facebookincubator/dns/dnsrocks/cgo-rocksdb"

	"golang.org/x/sync/errgroup"
)

// ballpark for external shard size
const estimatedKeyCount = 20000000

// template for SST file names
const templateSSTFileName = "%s/rdb%d.sst"

// minimum bucket size, in number of items
const minBucketSize = 30000

// full Bloom filter bits
const fullBloomFilterBits = 10

type bucket struct {
	startOffset, endOffset int
}

// Builder is specifically optimized for building a database from scratch.
// It has a number of optimizations that would not work on an already existing
// database, so it should not be used if the database already exist (the result
// will be undefined if so).
type Builder struct {
	db           DBI
	writeOptions *rocksdb.WriteOptions
	values       []keyValues
	buckets      []bucket
	path         string
	useHardlinks bool
}

// NewBuilder creates a new instance of Builder
func NewBuilder(path string, useHardlinks bool) (*Builder, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s directory does not exist: %w", path, err)
	}

	log.Println("Creating database", path)

	options := rocksdb.NewOptions()
	options.EnableCreateIfMissing()
	options.EnableErrorIfExists()
	options.SetParallelism(runtime.NumCPU())
	options.OptimizeLevelStyleCompaction(0)
	options.PrepareForBulkLoad()
	options.SetFullBloomFilter(fullBloomFilterBits)

	db, err := rocksdb.OpenDatabase(path, false, false, options)
	if err != nil {
		options.FreeOptions()
		return nil, err
	}
	// disable WAL, we don't care about data loss if we fail to do initial building
	writeOptions := rocksdb.NewWriteOptions(
		false, true, true, false, false,
	)
	return &Builder{
		db:           db,
		values:       make([]keyValues, 0, estimatedKeyCount),
		writeOptions: writeOptions,
		path:         path,
		useHardlinks: useHardlinks,
	}, nil
}

// FreeBuilder closes the database
func (b *Builder) FreeBuilder() {
	b.writeOptions.FreeWriteOptions()
	b.db.CloseDatabase()
}

// ScheduleAdd schedules addition of a multi-value pair of key and value
func (b *Builder) ScheduleAdd(key, value []byte) {
	b.values = append(
		b.values,
		keyValues{
			key:    copyBytes(key),
			values: [][]byte{copyBytes(value)},
		},
	)
}

// sort all values in binary order
func (b *Builder) sortDataset() {
	log.Println("Sorting ...")
	sort.Slice(
		b.values,
		func(i, j int) bool {
			return bytes.Compare(b.values[i].key, b.values[j].key) < 0
		},
	)
}

// createBuckets splits b.values between the maximum of maxBucketNum buckets; each bucket will contain at least minBucketSize elements,
// and all values with the same key will belong to the same bucket.
// Effectively, each bucket will contain a non-overlapping with other buckets set of sorted keys.
func (b *Builder) createBuckets(minBucketSize, maxBucketNum int) {
	var bucketEnd int
	log.Println("Creating buckets no smaller than", minBucketSize, "items each, and no more than", maxBucketNum, "buckets total")
	b.buckets = make([]bucket, 0, maxBucketNum)
	minInt := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}
	maxInt := func(a, b int) int {
		if a > b {
			return a
		}
		return b
	}
	bucketSize := maxInt(minBucketSize, len(b.values)/maxBucketNum)
	bucketStart := 0
	for i := 0; i < maxBucketNum; i++ {
		if i+1 == maxBucketNum {
			// last bucket should cover until everything until the very end of the dataset
			bucketEnd = len(b.values)
		} else {
			for bucketEnd = minInt(bucketStart+bucketSize, len(b.values)); bucketEnd < len(b.values); bucketEnd++ {
				// find where to slice: values with the same key should always get into the same bucket
				if !bytes.Equal(b.values[bucketEnd].key, b.values[bucketEnd-1].key) {
					break
				}
			}
		}
		b.buckets = append(
			b.buckets,
			bucket{
				startOffset: bucketStart,
				endOffset:   bucketEnd,
			},
		)
		if bucketEnd == len(b.values) {
			break
		}
		bucketStart = bucketEnd
	}
	log.Println("Created buckets:", b.buckets)
}

// saveBuckets saves each bucket in SST file for ingestion later
func (b *Builder) saveBuckets() ([]string, error) {
	startTime := time.Now()
	var g errgroup.Group
	sstFilePaths := make([]string, len(b.buckets))
	sstFileSizes := make([]uint64, len(b.buckets))
	sstFileKeyCount := make([]int, len(b.buckets))
	for i, bucket := range b.buckets {
		bucketNo := i
		bucketItems := b.values[bucket.startOffset:bucket.endOffset]
		g.Go(func() error {
			if len(bucketItems) == 0 {
				return fmt.Errorf("Assertion failed: bucket %d is empty", bucketNo)
			}
			writerName := fmt.Sprintf("w#%d", bucketNo)
			filePath := fmt.Sprintf(templateSSTFileName, b.path, bucketNo)
			sstFilePaths[bucketNo] = filePath
			log.Println(writerName, "saving", len(bucketItems), "values into", filePath)
			writer, err := rocksdb.CreateSSTFileWriter(filePath)
			if err != nil {
				return fmt.Errorf("%s: error creating writer to %s - %w", writerName, filePath, err)
			}
			keyCount := 0
			var prevKey []byte
			accumulator := make([]byte, 0, 1024)
			for _, item := range bucketItems {
				if (prevKey != nil) && (!bytes.Equal(item.key, prevKey)) {
					if err := writer.Put(prevKey, accumulator); err != nil {
						return fmt.Errorf("%s: error writing to %s - %w", writerName, filePath, err)
					}
					accumulator = accumulator[0:0]
					keyCount++
				}
				accumulator = appendValues(accumulator, item.values)
				prevKey = item.key
			}
			// flush
			if err := writer.Put(prevKey, accumulator); err != nil {
				return fmt.Errorf("%s: error writing to %s - %w", writerName, filePath, err)
			}
			keyCount++
			sstFileSizes[bucketNo] = writer.GetFileSize()
			sstFileKeyCount[bucketNo] = keyCount
			log.Println(writerName, "wrote", sstFileSizes[bucketNo], "bytes to", filePath, "finishing write...")
			if err := writer.Finish(); err != nil {
				return fmt.Errorf("%s: error finishing writer to %s - %w", writerName, filePath, err)
			}
			writer.CloseWriter()
			return nil
		})
	}

	// process errors, immediately return if there are any
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// count stats
	totalSizeBytes := 0
	for _, sizeBytes := range sstFileSizes {
		totalSizeBytes += int(sizeBytes)
	}
	totalKeyCount := 0
	for _, keyCount := range sstFileKeyCount {
		totalKeyCount += keyCount
	}
	elapsed := float64(time.Since(startTime)/time.Millisecond) / 1000.0
	log.Printf(
		"%d buckets saved, %d keys in %.3f seconds, %.2f keys per second, %.1f MiB total",
		len(b.buckets), totalKeyCount, elapsed,
		float64(totalKeyCount)/elapsed, float64(totalSizeBytes)/(1024.0*1024.0),
	)
	return sstFilePaths, nil
}

func (b *Builder) ingestFiles(sstFilePaths []string) error {
	if err := b.db.IngestSSTFiles(sstFilePaths, b.useHardlinks); err != nil {
		return fmt.Errorf("error ingesting files: %w", err)
	}
	log.Println("Ingesting done, cleanup")
	if !b.useHardlinks {
		for _, path := range sstFilePaths {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("error removing file %s: %w", path, err)
			}
		}
	}
	return nil
}

// Execute builds the database from accumulated dataset
func (b *Builder) Execute() error {
	b.sortDataset()
	b.createBuckets(minBucketSize, runtime.NumCPU())

	sstFilePaths, err := b.saveBuckets()
	if err != nil {
		return err
	}

	err = b.ingestFiles(sstFilePaths)
	if err != nil {
		return err
	}

	log.Println("Building done")
	return nil
}
