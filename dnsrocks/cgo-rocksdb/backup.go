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
#include <stdlib.h> // for free()

const int BACKUP_BOOL_INT_TRUE = 1;
*/
import "C"

import (
	"errors"
	"unsafe"
)

// BackupEngineInfo represents the information about the backups
type BackupEngineInfo struct {
	cInfo *C.rocksdb_backup_engine_info_t
}

// GetCount gets the number backups available.
func (bi *BackupEngineInfo) GetCount() int {
	return int(C.rocksdb_backup_engine_info_count(bi.cInfo))
}

// GetTimestamp gets the timestamp at which the backup was taken.
func (bi *BackupEngineInfo) GetTimestamp(index int) int64 {
	return int64(C.rocksdb_backup_engine_info_timestamp(bi.cInfo, C.int(index)))
}

// GetBackupID gets an id that uniquely identifies a backup
func (bi *BackupEngineInfo) GetBackupID(index int) int64 {
	return int64(C.rocksdb_backup_engine_info_backup_id(bi.cInfo, C.int(index)))
}

// GetSize get the size of the backup in bytes.
func (bi *BackupEngineInfo) GetSize(index int) int64 {
	return int64(C.rocksdb_backup_engine_info_size(bi.cInfo, C.int(index)))
}

// GetNumFiles gets the number of files in the backup.
func (bi *BackupEngineInfo) GetNumFiles(index int) int32 {
	return int32(C.rocksdb_backup_engine_info_number_files(bi.cInfo, C.int(index)))
}

// Free frees up memory allocated by BackupEngine.GetInfo().
func (bi *BackupEngineInfo) Free() {
	C.rocksdb_backup_engine_info_destroy(bi.cInfo)
}

// BackupEngine is a front-end for backup / restore operations
type BackupEngine struct {
	cEngine *C.rocksdb_backup_engine_t
	options *Options
}

// NewBackupEngine creates an instance of BackupEngine
func NewBackupEngine(backupPath string) (*BackupEngine, error) {
	var cError *C.char
	cBackupPath := C.CString(backupPath)
	defer C.free(unsafe.Pointer(cBackupPath))
	options := NewOptions()

	cEngine := C.rocksdb_backup_engine_open(options.cOptions, cBackupPath, &cError)

	if cError != nil {
		options.FreeOptions()
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return nil, errors.New(C.GoString(cError))
	}

	return &BackupEngine{
		cEngine: cEngine,
		options: options,
	}, nil
}

// BackupDatabase will backup the database; flushMemtable parameter will force
// flushing memtable before starting the backup, so the log files
// will not be necessary for restore (and will not be copied to the backup
// directory). If flushMemtable is not set, then the log files will be
// copied to the backup directory.
func (e *BackupEngine) BackupDatabase(db *RocksDB, flushMemtable bool) error {
	var cError *C.char

	C.rocksdb_backup_engine_create_new_backup_flush(
		e.cEngine, db.cDB, BoolToChar(flushMemtable), &cError,
	)

	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}

	return nil
}

// PurgeOldBackups removes all but the last numBackupsToKeep from the backup directory
func (e *BackupEngine) PurgeOldBackups(numBackupsToKeep uint32) error {
	var cError *C.char

	C.rocksdb_backup_engine_purge_old_backups(e.cEngine, C.uint(numBackupsToKeep), &cError)

	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}

	return nil
}

// RestoreFromLastBackup restores the last created backup to destPath directory,
// if keepLogFiles is true - it will not overwrite WAL in destPath (which is most
// of a time a strange thing to do)
func (e *BackupEngine) RestoreFromLastBackup(destPath string, keepLogFiles bool) error {
	// create restore options
	var cRestoreOptions *C.rocksdb_restore_options_t = C.rocksdb_restore_options_create()
	defer C.rocksdb_restore_options_destroy(cRestoreOptions)

	if keepLogFiles {
		C.rocksdb_restore_options_set_keep_log_files(cRestoreOptions, C.BACKUP_BOOL_INT_TRUE)
	}

	cDestPath := C.CString(destPath)
	defer C.free(unsafe.Pointer(cDestPath))

	var cError *C.char
	C.rocksdb_backup_engine_restore_db_from_latest_backup(
		e.cEngine, cDestPath, cDestPath, cRestoreOptions, &cError,
	)

	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}

	return nil
}

// GetInfo gets an object that gives information about
// the backups that have already been taken
func (e *BackupEngine) GetInfo() *BackupEngineInfo {
	return &BackupEngineInfo{
		cInfo: C.rocksdb_backup_engine_get_backup_info(e.cEngine),
	}
}

// FreeBackupEngine frees up the memory allocated by NewBackupEngine()
func (e *BackupEngine) FreeBackupEngine() {
	C.rocksdb_backup_engine_close(e.cEngine)
	e.options.FreeOptions()
}
