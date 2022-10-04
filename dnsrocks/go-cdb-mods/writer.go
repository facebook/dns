package cdb

import (
	"bufio"
	"hash"
	"io"
	"os"
)

// Writer is the interface to be used by constant database writers. Its methods
// used to be the same present in mcdb.Writer so that could also satisfy this interface.
type Writer interface {
	// Put adds a new key/value pair associated with the given tag,
	Put(key, value []byte) error

	// Close closes the constant database, flushing all data to disk.
	Close() error
}

// NewWriter returns a constant database writer that uses go-cdb as its
// implementation.
func NewWriter(fileName string) (Writer, error) {
	f, err := os.Create(fileName)
	if err != nil {
		return nil, err
	}

	w := io.WriteSeeker(f)
	if _, err := w.Seek(int64(headerSize), 0); err != nil {
		return nil, err
	}

	newWriter := &writer{
		make([]byte, 8),
		w,
		bufio.NewWriter(f),
		cdbHash(),
		nil,
		make(map[uint32][]slot),
		headerSize,
	}

	newWriter.hw = io.MultiWriter(newWriter.hash, newWriter.wb)

	return newWriter, nil
}

// go-cdb base Writer implementation.

type writer struct {
	buf     []byte
	w       io.WriteSeeker
	wb      *bufio.Writer
	hash    hash.Hash32
	hw      io.Writer
	htables map[uint32][]slot
	pos     uint32
}

func (w *writer) Put(key, value []byte) error {
	klen, dlen := uint32(len(key)), uint32(len(value))
	writeNums(w.wb, klen, dlen, w.buf)

	w.hash.Reset()

	// Write key (with tag) and update hash.
	_, err := w.hw.Write(key)
	if err != nil {
		return err
	}

	// Write value.
	_, err = w.wb.Write(value)
	if err != nil {
		return err
	}

	// Compute hash and update hash tables.
	h := w.hash.Sum32()
	tableNum := h % 256
	w.htables[tableNum] = append(w.htables[tableNum], slot{h, w.pos})

	// Move position.
	w.pos += 8 + klen + dlen

	return nil
}

func (w *writer) Close() error {
	maxSlots := 0
	for _, slots := range w.htables {
		if len(slots) > maxSlots {
			maxSlots = len(slots)
		}
	}
	slotTable := make([]slot, maxSlots*2)

	header := make([]byte, headerSize)

	// Write hash tables.
	for i := uint32(0); i < 256; i++ {
		slots := w.htables[i]
		if slots == nil {
			putNum(header[i*8:], w.pos)
			continue
		}

		nslots := uint32(len(slots) * 2)
		hashSlotTable := slotTable[:nslots]

		// Reset table slots.
		for j := 0; j < len(hashSlotTable); j++ {
			hashSlotTable[j].h = 0
			hashSlotTable[j].pos = 0
		}

		for _, slot := range slots {
			slotPos := (slot.h / 256) % nslots
			for hashSlotTable[slotPos].pos != 0 {
				slotPos++
				if slotPos == uint32(len(hashSlotTable)) {
					slotPos = 0
				}
			}
			hashSlotTable[slotPos] = slot
		}

		if err := writeSlots(w.wb, hashSlotTable, w.buf); err != nil {
			return err
		}

		putNum(header[i*8:], w.pos)
		putNum(header[i*8+4:], nslots)
		w.pos += 8 * nslots
	}

	if err := w.wb.Flush(); err != nil {
		return err
	}

	if _, err := w.w.Seek(0, 0); err != nil {
		return err
	}

	_, err := w.w.Write(header)

	return err
}
