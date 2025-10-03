package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/rivetq/rivetq/internal/util"
)

const (
	// DefaultSegmentSize is the default max size for a WAL segment (64MB)
	DefaultSegmentSize = 64 * 1024 * 1024
	// SegmentFilePattern for naming segments
	SegmentFilePattern = "%06d.wal"
)

// Segment represents a single WAL segment file
type Segment struct {
	mu       sync.RWMutex
	id       uint64
	path     string
	file     *os.File
	writer   *bufio.Writer
	size     int64
	maxSize  int64
	fsync    bool
	readOnly bool
}

// NewSegment creates a new WAL segment
func NewSegment(dir string, id uint64, maxSize int64, fsync bool) (*Segment, error) {
	path := filepath.Join(dir, fmt.Sprintf(SegmentFilePattern, id))

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat segment file: %w", err)
	}

	return &Segment{
		id:      id,
		path:    path,
		file:    file,
		writer:  bufio.NewWriter(file),
		size:    stat.Size(),
		maxSize: maxSize,
		fsync:   fsync,
	}, nil
}

// OpenSegmentReadOnly opens a segment for reading only
func OpenSegmentReadOnly(path string) (*Segment, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat segment file: %w", err)
	}

	return &Segment{
		path:     path,
		file:     file,
		size:     stat.Size(),
		readOnly: true,
	}, nil
}

// Write writes a record to the segment
// Format: [length:4][crc32:4][data...]
func (s *Segment) Write(record *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return fmt.Errorf("segment is read-only")
	}

	data, err := record.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Calculate checksum
	checksum := util.Checksum(data)

	// Write length
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := s.writer.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	// Write checksum
	crcBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(crcBuf, checksum)
	if _, err := s.writer.Write(crcBuf); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	// Write data
	if _, err := s.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Flush and optionally fsync
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	if s.fsync {
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("failed to fsync: %w", err)
		}
	}

	s.size += int64(8 + len(data))
	return nil
}

// IsFull checks if segment has reached max size
func (s *Segment) IsFull() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.size >= s.maxSize
}

// Size returns current segment size
func (s *Segment) Size() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.size
}

// ID returns segment ID
func (s *Segment) ID() uint64 {
	return s.id
}

// Close closes the segment
func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.readOnly && s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}

	if s.file != nil {
		return s.file.Close()
	}

	return nil
}

// Reader returns a new reader for this segment
func (s *Segment) Reader() (*SegmentReader, error) {
	return NewSegmentReader(s.path)
}

// SegmentReader reads records from a segment
type SegmentReader struct {
	file   *os.File
	reader *bufio.Reader
	offset int64
}

// NewSegmentReader creates a new segment reader
func NewSegmentReader(path string) (*SegmentReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment: %w", err)
	}

	return &SegmentReader{
		file:   file,
		reader: bufio.NewReader(file),
	}, nil
}

// Read reads the next record from segment
func (sr *SegmentReader) Read() (*Record, error) {
	// Read length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(sr.reader, lenBuf); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read length: %w", err)
	}
	length := binary.LittleEndian.Uint32(lenBuf)

	// Read checksum
	crcBuf := make([]byte, 4)
	if _, err := io.ReadFull(sr.reader, crcBuf); err != nil {
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}
	expectedCRC := binary.LittleEndian.Uint32(crcBuf)

	// Read data
	data := make([]byte, length)
	if _, err := io.ReadFull(sr.reader, data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	// Verify checksum
	if !util.VerifyChecksum(data, expectedCRC) {
		return nil, ErrCorruptedData
	}

	// Unmarshal record
	record := &Record{}
	if err := record.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %w", err)
	}

	sr.offset += int64(8 + length)
	return record, nil
}

// Close closes the reader
func (sr *SegmentReader) Close() error {
	if sr.file != nil {
		return sr.file.Close()
	}
	return nil
}
