package wal

import (
	"encoding/binary"
	"errors"
	"time"
)

// RecordType defines the type of WAL record
type RecordType uint8

const (
	RecordTypeEnqueue RecordType = iota + 1
	RecordTypeAck
	RecordTypeNack
	RecordTypeRequeue
	RecordTypeTombstone
)

var (
	ErrInvalidRecord = errors.New("invalid record")
	ErrCorruptedData = errors.New("corrupted data")
)

// Record represents a WAL entry
type Record struct {
	Type     RecordType
	Queue    string
	JobID    string
	Payload  []byte
	Headers  map[string]string
	Priority uint8
	Tries    uint32
	MaxRetries uint32
	ETA      time.Time // Execute Time After - for delayed jobs
	LeaseID  string
	Reason   string // For Nack
}

// Marshal serializes a record to bytes
// Format: [type:1][queue_len:2][queue][job_id_len:2][job_id][priority:1][tries:4][max_retries:4]
//         [eta_unix_ms:8][payload_len:4][payload][headers_count:2][headers...][lease_id_len:2][lease_id][reason_len:2][reason]
func (r *Record) Marshal() ([]byte, error) {
	// Estimate size
	size := 1 + 2 + len(r.Queue) + 2 + len(r.JobID) + 1 + 4 + 4 + 8 + 4 + len(r.Payload) + 2

	for k, v := range r.Headers {
		size += 2 + len(k) + 2 + len(v)
	}
	size += 2 + len(r.LeaseID) + 2 + len(r.Reason)

	buf := make([]byte, size)
	offset := 0

	// Type
	buf[offset] = byte(r.Type)
	offset++

	// Queue
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(r.Queue)))
	offset += 2
	copy(buf[offset:], r.Queue)
	offset += len(r.Queue)

	// JobID
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(r.JobID)))
	offset += 2
	copy(buf[offset:], r.JobID)
	offset += len(r.JobID)

	// Priority
	buf[offset] = r.Priority
	offset++

	// Tries
	binary.LittleEndian.PutUint32(buf[offset:], r.Tries)
	offset += 4

	// MaxRetries
	binary.LittleEndian.PutUint32(buf[offset:], r.MaxRetries)
	offset += 4

	// ETA (unix milliseconds)
	etaMs := r.ETA.UnixMilli()
	binary.LittleEndian.PutUint64(buf[offset:], uint64(etaMs))
	offset += 8

	// Payload
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(r.Payload)))
	offset += 4
	copy(buf[offset:], r.Payload)
	offset += len(r.Payload)

	// Headers
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(r.Headers)))
	offset += 2
	for k, v := range r.Headers {
		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(k)))
		offset += 2
		copy(buf[offset:], k)
		offset += len(k)

		binary.LittleEndian.PutUint16(buf[offset:], uint16(len(v)))
		offset += 2
		copy(buf[offset:], v)
		offset += len(v)
	}

	// LeaseID
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(r.LeaseID)))
	offset += 2
	copy(buf[offset:], r.LeaseID)
	offset += len(r.LeaseID)

	// Reason
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(r.Reason)))
	offset += 2
	copy(buf[offset:], r.Reason)
	offset += len(r.Reason)

	return buf[:offset], nil
}

// Unmarshal deserializes a record from bytes
func (r *Record) Unmarshal(data []byte) error {
	if len(data) < 1 {
		return ErrInvalidRecord
	}

	offset := 0

	// Type
	r.Type = RecordType(data[offset])
	offset++

	// Queue
	if offset+2 > len(data) {
		return ErrInvalidRecord
	}
	queueLen := binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	if offset+int(queueLen) > len(data) {
		return ErrInvalidRecord
	}
	r.Queue = string(data[offset : offset+int(queueLen)])
	offset += int(queueLen)

	// JobID
	if offset+2 > len(data) {
		return ErrInvalidRecord
	}
	jobIDLen := binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	if offset+int(jobIDLen) > len(data) {
		return ErrInvalidRecord
	}
	r.JobID = string(data[offset : offset+int(jobIDLen)])
	offset += int(jobIDLen)

	// Priority
	if offset+1 > len(data) {
		return ErrInvalidRecord
	}
	r.Priority = data[offset]
	offset++

	// Tries
	if offset+4 > len(data) {
		return ErrInvalidRecord
	}
	r.Tries = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// MaxRetries
	if offset+4 > len(data) {
		return ErrInvalidRecord
	}
	r.MaxRetries = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// ETA
	if offset+8 > len(data) {
		return ErrInvalidRecord
	}
	etaMs := int64(binary.LittleEndian.Uint64(data[offset:]))
	r.ETA = time.UnixMilli(etaMs)
	offset += 8

	// Payload
	if offset+4 > len(data) {
		return ErrInvalidRecord
	}
	payloadLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(payloadLen) > len(data) {
		return ErrInvalidRecord
	}
	r.Payload = make([]byte, payloadLen)
	copy(r.Payload, data[offset:offset+int(payloadLen)])
	offset += int(payloadLen)

	// Headers
	if offset+2 > len(data) {
		return ErrInvalidRecord
	}
	headersCount := binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	r.Headers = make(map[string]string, headersCount)
	for i := 0; i < int(headersCount); i++ {
		// Key
		if offset+2 > len(data) {
			return ErrInvalidRecord
		}
		keyLen := binary.LittleEndian.Uint16(data[offset:])
		offset += 2
		if offset+int(keyLen) > len(data) {
			return ErrInvalidRecord
		}
		key := string(data[offset : offset+int(keyLen)])
		offset += int(keyLen)

		// Value
		if offset+2 > len(data) {
			return ErrInvalidRecord
		}
		valLen := binary.LittleEndian.Uint16(data[offset:])
		offset += 2
		if offset+int(valLen) > len(data) {
			return ErrInvalidRecord
		}
		val := string(data[offset : offset+int(valLen)])
		offset += int(valLen)

		r.Headers[key] = val
	}

	// LeaseID
	if offset+2 > len(data) {
		return ErrInvalidRecord
	}
	leaseIDLen := binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	if offset+int(leaseIDLen) > len(data) {
		return ErrInvalidRecord
	}
	r.LeaseID = string(data[offset : offset+int(leaseIDLen)])
	offset += int(leaseIDLen)

	// Reason
	if offset+2 > len(data) {
		return ErrInvalidRecord
	}
	reasonLen := binary.LittleEndian.Uint16(data[offset:])
	offset += 2
	if offset+int(reasonLen) > len(data) {
		return ErrInvalidRecord
	}
	r.Reason = string(data[offset : offset+int(reasonLen)])
	offset += int(reasonLen)

	return nil
}
