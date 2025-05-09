/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	rocksdb "github.com/facebook/dns/dnsrocks/cgo-rocksdb"
	"github.com/facebook/dns/dnsrocks/dnsdata/rdb"
)

const (
	actionBackup  = "backup"
	actionRestore = "restore"
	actionInfo    = "info"
)

func assertDirExists(path string) error {
	fileInfo, err := os.Stat(path)
	if err == nil && !fileInfo.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", path)
	}
	return err
}

func backup(dbPath, backupPath string, createIfMissing bool) error {
	if createIfMissing {
		if err := os.MkdirAll(backupPath, 0755); err != nil {
			return fmt.Errorf("failed to create backup dir: %w", err)
		}
	} else {
		if err := assertDirExists(backupPath); err != nil {
			return err
		}
	}
	return rdb.Backup(dbPath, backupPath)
}

func restore(dbPath, backupPath string, createIfMissing bool) error {
	if createIfMissing {
		if err := os.MkdirAll(dbPath, 0755); err != nil {
			return fmt.Errorf("failed to create db dir: %w", err)
		}
	} else {
		if err := assertDirExists(dbPath); err != nil {
			return err
		}
	}
	return rdb.Restore(dbPath, backupPath)
}

func info(backupPath string) error {
	backupEngine, err := rocksdb.NewBackupEngine(backupPath)
	if err != nil {
		return fmt.Errorf("error creating backup engine: %w", err)
	}
	defer backupEngine.FreeBackupEngine()
	info := backupEngine.GetInfo()
	defer info.Free()
	cnt := info.GetCount()
	fmt.Printf("Backup directory %s contains %d backups\n", backupPath, cnt)
	for i := 0; i < cnt; i++ {
		fmt.Printf("Backup %d from %v\n", i, time.Unix(info.GetTimestamp(i), 0))
		fmt.Printf("\t size: %dMb\n", info.GetSize(i)/1024/1024)
		fmt.Printf("\t num files: %d\n", info.GetNumFiles(i))
	}
	return nil
}

func main() {
	dbDir := flag.String("db", "", "Path to RocksDB directory")
	backupDir := flag.String("backup", "", "Path to backup directory")
	action := flag.String("action", "", `one of the following actions:
* backup - create a backup of DB
* restore - restore latest backup
* info - display information about backups in directory
`)
	create := flag.Bool("create", true, "create destination directory if missing")
	flag.Parse()

	if *action == "" {
		log.Fatalf("no action specified")
	}
	if *action == actionInfo {
		if *backupDir == "" {
			log.Fatal("Backup directory needs to be specified")
		}
	} else if *dbDir == "" || *backupDir == "" {
		log.Fatal("Both directories need to be specified")
	}

	switch *action {
	case actionBackup:
		if err := backup(*dbDir, *backupDir, *create); err != nil {
			log.Fatalf("Failed to backup: %v", err)
		}
	case actionRestore:
		if err := restore(*dbDir, *backupDir, *create); err != nil {
			log.Fatalf("Failed to restore: %v", err)
		}
	case actionInfo:
		if err := info(*backupDir); err != nil {
			log.Fatalf("Failed to get info: %v", err)
		}
	default:
		log.Fatalf("unknown action '%s'", *action)
	}
}
