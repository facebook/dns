//go:build darwin || linux
// +build darwin linux

package portablemmap

import (
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

func Prefault(mmappedData []byte) {
	// Prefaults mmaped file so it is preloaded in the filesystem cache.
	// Note that you should *NOT* call this if the CDB file is bigger than
	// the available physical memory.
	sliceHeader := *(*reflect.SliceHeader)(unsafe.Pointer(&mmappedData))
	syscall.Syscall(syscall.SYS_MSYNC, uintptr(sliceHeader.Data),
		uintptr(sliceHeader.Len), uintptr(
			syscall.MADV_WILLNEED|syscall.MADV_RANDOM))
}

func Mmap(f *os.File) ([]byte, error) {
	// Get file info. We need its size later to map it entirely.
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Mmap file and return.
	return syscall.Mmap(int(f.Fd()), 0, int(fi.Size()),
		syscall.PROT_READ, syscall.MAP_SHARED)
}

func Munmap(mmappedData []byte) error {
	return syscall.Munmap(mmappedData)
}
