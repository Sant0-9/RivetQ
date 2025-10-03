package util

import (
	"encoding/binary"
	"hash/crc32"
)

var (
	// Castagnoli polynomial for CRC32C
	crc32cTable = crc32.MakeTable(crc32.Castagnoli)
)

// Checksum computes CRC32C checksum of data
func Checksum(data []byte) uint32 {
	return crc32.Checksum(data, crc32cTable)
}

// VerifyChecksum verifies data against expected checksum
func VerifyChecksum(data []byte, expected uint32) bool {
	return Checksum(data) == expected
}

// AppendChecksum appends CRC32C checksum to data
func AppendChecksum(data []byte) []byte {
	checksum := Checksum(data)
	result := make([]byte, len(data)+4)
	copy(result, data)
	binary.LittleEndian.PutUint32(result[len(data):], checksum)
	return result
}
