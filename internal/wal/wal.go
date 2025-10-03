package wal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// WAL manages write-ahead log segments
type WAL struct {
	mu            sync.RWMutex
	dir           string
	segments      []*Segment
	activeSegment *Segment
	nextSegmentID uint64
	segmentSize   int64
	fsync         bool
}

// Config for WAL
type Config struct {
	Dir         string
	SegmentSize int64
	Fsync       bool
}

// New creates a new WAL instance
func New(cfg Config) (*WAL, error) {
	if cfg.SegmentSize == 0 {
		cfg.SegmentSize = DefaultSegmentSize
	}

	// Create directory if not exists
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	wal := &WAL{
		dir:         cfg.Dir,
		segments:    make([]*Segment, 0),
		segmentSize: cfg.SegmentSize,
		fsync:       cfg.Fsync,
	}

	// Load existing segments
	if err := wal.loadSegments(); err != nil {
		return nil, fmt.Errorf("failed to load segments: %w", err)
	}

	// Create initial segment if none exist
	if wal.activeSegment == nil {
		if err := wal.createSegment(); err != nil {
			return nil, fmt.Errorf("failed to create initial segment: %w", err)
		}
	}

	return wal, nil
}

// loadSegments loads existing segment files from disk
func (w *WAL) loadSegments() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}

	segmentIDs := make([]uint64, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wal") {
			continue
		}

		// Parse segment ID from filename
		name := strings.TrimSuffix(entry.Name(), ".wal")
		id, err := strconv.ParseUint(name, 10, 64)
		if err != nil {
			log.Warn().Str("file", entry.Name()).Msg("invalid segment filename")
			continue
		}

		segmentIDs = append(segmentIDs, id)
	}

	if len(segmentIDs) == 0 {
		return nil
	}

	// Sort by ID
	sort.Slice(segmentIDs, func(i, j int) bool {
		return segmentIDs[i] < segmentIDs[j]
	})

	// Open segments
	for _, id := range segmentIDs {
		segment, err := NewSegment(w.dir, id, w.segmentSize, w.fsync)
		if err != nil {
			return fmt.Errorf("failed to open segment %d: %w", id, err)
		}
		w.segments = append(w.segments, segment)
	}

	// Set active segment to the last one
	w.activeSegment = w.segments[len(w.segments)-1]
	w.nextSegmentID = segmentIDs[len(segmentIDs)-1] + 1

	return nil
}

// createSegment creates a new segment
func (w *WAL) createSegment() error {
	segment, err := NewSegment(w.dir, w.nextSegmentID, w.segmentSize, w.fsync)
	if err != nil {
		return err
	}

	w.segments = append(w.segments, segment)
	w.activeSegment = segment
	w.nextSegmentID++

	return nil
}

// Write writes a record to the WAL
func (w *WAL) Write(record *Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we need to rotate segment
	if w.activeSegment.IsFull() {
		if err := w.createSegment(); err != nil {
			return fmt.Errorf("failed to create new segment: %w", err)
		}
	}

	if err := w.activeSegment.Write(record); err != nil {
		return fmt.Errorf("failed to write to segment: %w", err)
	}

	return nil
}

// Replay reads all records from WAL and calls the callback for each
func (w *WAL) Replay(callback func(*Record) error) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, segment := range w.segments {
		reader, err := segment.Reader()
		if err != nil {
			return fmt.Errorf("failed to create reader for segment %d: %w", segment.ID(), err)
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err == ErrCorruptedData {
				log.Warn().Uint64("segment", segment.ID()).Msg("corrupted record, skipping rest of segment")
				break
			}
			if err != nil {
				reader.Close()
				return fmt.Errorf("failed to read from segment %d: %w", segment.ID(), err)
			}

			if err := callback(record); err != nil {
				reader.Close()
				return fmt.Errorf("callback failed: %w", err)
			}
		}

		reader.Close()
	}

	return nil
}

// Compact removes old segments and compacts data
func (w *WAL) Compact(activeJobIDs map[string]bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Keep only the last segment and create a new compacted segment
	if len(w.segments) <= 1 {
		return nil // Nothing to compact
	}

	log.Info().Int("segments", len(w.segments)).Msg("starting WAL compaction")

	// Create a new temporary segment for compacted data
	tempSegment, err := NewSegment(w.dir, w.nextSegmentID, w.segmentSize, w.fsync)
	if err != nil {
		return fmt.Errorf("failed to create compaction segment: %w", err)
	}

	// Replay all segments except the active one, writing only active jobs
	for i := 0; i < len(w.segments)-1; i++ {
		segment := w.segments[i]
		reader, err := segment.Reader()
		if err != nil {
			return fmt.Errorf("failed to create reader: %w", err)
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err == ErrCorruptedData {
				break
			}
			if err != nil {
				reader.Close()
				return err
			}

			// Only write enqueue records for jobs that are still active
			if record.Type == RecordTypeEnqueue && activeJobIDs[record.JobID] {
				if err := tempSegment.Write(record); err != nil {
					reader.Close()
					return err
				}
			}
		}
		reader.Close()
	}

	tempSegment.Close()

	// Remove old segments
	for i := 0; i < len(w.segments)-1; i++ {
		segment := w.segments[i]
		segment.Close()
		os.Remove(segment.path)
	}

	// Update segments list
	w.segments = []*Segment{w.segments[len(w.segments)-1]} // Keep active segment

	// Rename compacted segment if it has data
	if tempSegment.Size() > 0 {
		compactedSegment, err := NewSegment(w.dir, w.nextSegmentID, w.segmentSize, w.fsync)
		if err != nil {
			return err
		}
		w.segments = append([]*Segment{compactedSegment}, w.segments...)
		w.nextSegmentID++
	} else {
		// Remove empty compacted segment
		os.Remove(tempSegment.path)
	}

	log.Info().Int("segments_after", len(w.segments)).Msg("WAL compaction completed")
	return nil
}

// Close closes all segments
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, segment := range w.segments {
		if err := segment.Close(); err != nil {
			return err
		}
	}

	return nil
}

// SegmentCount returns the number of segments
func (w *WAL) SegmentCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.segments)
}

// TotalSize returns total size of all segments
func (w *WAL) TotalSize() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var total int64
	for _, seg := range w.segments {
		total += seg.Size()
	}
	return total
}
