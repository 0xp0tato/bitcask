package bitcask

import (
	"encoding/binary"
	"hash/crc32"
	"time"
)

func encode(key, val []byte) []byte {
	keySize := len(key)
	valSize := len(val)
	buffer := make([]byte, headerSize+valSize+keySize)
	timestamp := time.Now().Unix()

	// enter timestamp to buffer
	binary.BigEndian.PutUint32(buffer[4:8], uint32(timestamp))
	// enter keySize to buffer
	binary.BigEndian.PutUint32(buffer[8:12], uint32(keySize))
	// enter valSize to buffer
	binary.BigEndian.PutUint32(buffer[12:16], uint32(valSize))

	copy(buffer[headerSize:headerSize+keySize], key)
	copy(buffer[headerSize+keySize:], val)

	checksum := crc32.ChecksumIEEE(buffer[4:])
	binary.BigEndian.PutUint32(buffer[0:4], checksum)

	return buffer
}
