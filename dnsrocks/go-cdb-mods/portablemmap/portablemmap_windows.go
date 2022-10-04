package portablemmap

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"syscall"
	"unsafe"
)

var handleLock sync.Mutex
var handleMap = map[uintptr]syscall.Handle{}

func Prefault(mmapedData []byte) {
	// This is a no-op on Windows.
}

func Mmap(f *os.File) ([]byte, error) {
	// Stat the file so we can know its size. We always maps the entire
	// file.
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// We only care about read only access.
	flProtect := uint32(syscall.PAGE_READONLY)
	dwDesiredAccess := uint32(syscall.FILE_MAP_READ)

	// Compute high and low bits of file size.
	maxSizeHigh := uint32(fi.Size() >> 32)
	maxSizeLow := uint32(fi.Size() & 0xFFFFFFFF)

	// Create file mapping.
	h, err := syscall.CreateFileMapping(syscall.Handle(uintptr(f.Fd())),
		nil, flProtect, maxSizeHigh, maxSizeLow, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}

	// Compute high and low bits of offset into file.
	fileOffsetHigh := uint32(0)
	fileOffsetLow := uint32(0)

	// Create actual view into the mmapped file.
	addr, err := syscall.MapViewOfFile(h, dwDesiredAccess, fileOffsetHigh,
		fileOffsetLow, uintptr(fi.Size()))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}

	// Add handle to our map. We need it for unmmapping later.
	handleLock.Lock()
	handleMap[addr] = h
	handleLock.Unlock()

	// Create the slice we will use for keeping track of the data.
	mmappedData := []byte{}

	// Force slice to cover our mmaped region.
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mmappedData))
	sliceHeader.Data = addr
	sliceHeader.Len = int(fi.Size())
	sliceHeader.Cap = int(fi.Size())

	return mmappedData, nil
}

func Munmap(mmappedData []byte) error {
	sliceHeader := *(*reflect.SliceHeader)(unsafe.Pointer(&mmappedData))

	err := syscall.UnmapViewOfFile(sliceHeader.Data)
	if err != nil {
		return err
	}

	handleLock.Lock()
	defer handleLock.Unlock()
	handle, ok := handleMap[sliceHeader.Data]
	if !ok {
		return fmt.Errorf("can't find handle for mapped address")
	}

	delete(handleMap, sliceHeader.Data)

	err = syscall.CloseHandle(syscall.Handle(handle))

	return os.NewSyscallError("CloseHandle", err)
}
