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

package rdb

import (
	"fmt"

	rocksdb "github.com/facebook/dns/dnsrocks/cgo-rocksdb"
)

// Backup creates new backup
func Backup(dbPath, backupPath string) error {
	backupEngine, err := rocksdb.NewBackupEngine(backupPath)
	if err != nil {
		return fmt.Errorf("error creating backup engine: %w", err)
	}
	defer backupEngine.FreeBackupEngine()
	options := rocksdb.NewOptions()
	db, err := rocksdb.OpenDatabase(dbPath, true, false, options)
	if err != nil {
		options.FreeOptions()
		return fmt.Errorf("cannot create database: %w", err)
	}
	defer db.CloseDatabase()
	err = backupEngine.BackupDatabase(db, false)
	if err != nil {
		return fmt.Errorf("error backing up database %s: %w", dbPath, err)
	}
	return nil
}

// Restore restores data from backup
func Restore(dbPath, backupPath string) error {
	backupEngine, err := rocksdb.NewBackupEngine(backupPath)
	if err != nil {
		return fmt.Errorf("error creating backup engine: %w", err)
	}
	defer backupEngine.FreeBackupEngine()
	return backupEngine.RestoreFromLastBackup(dbPath, false)
}
