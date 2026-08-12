package bitcask

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

func decode(buffer []byte) (key []byte, val []byte, timestamp uint32, err error) {

	crc := binary.BigEndian.Uint32(buffer)
	timestamp = binary.BigEndian.Uint32(buffer[4:8])
	keySize := binary.BigEndian.Uint32(buffer[8:12])
	valSize := binary.BigEndian.Uint32(buffer[12:16])

	expectedCrc := crc32.ChecksumIEEE(buffer[4:])
	if crc != expectedCrc {
		return nil, nil, 0, errors.New("checksum mismatch: corrupted record")
	}

	key = buffer[headerSize : headerSize+int(keySize)]
	val = buffer[headerSize+int(keySize) : headerSize+int(keySize)+int(valSize)]

	return key, val, timestamp, nil

}
