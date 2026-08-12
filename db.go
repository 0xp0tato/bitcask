package bitcask

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type DB struct {
	dir        string
	files      map[int]*logfile
	activeId   int
	nextFileId int
	index      *Index
	mu         sync.RWMutex
}

func Open(path string) (*DB, error) {

	err := os.MkdirAll(path, 0750)
	if err != nil {
		return nil, err
	}

	fileIds, err := discoverFileIDs(path)
	if err != nil {
		return nil, err
	}

	db := &DB{
		files: make(map[int]*logfile),
		dir:   path,
		index: NewIndex(),
	}

	if len(fileIds) == 0 {
		filePath := filepath.Join(path, "0.log")
		db.activeId = 0
		db.nextFileId = 1

		lf, err := OpenLogFile(filePath)
		if err != nil {
			return nil, err
		}

		if err := db.readfile(lf, db.activeId, 0); err != nil {
			return nil, err
		}

		db.files[db.activeId] = lf

	} else {
		maxId := 0
		for i := range len(fileIds) {
			fileId := fileIds[i]
			fileName := strconv.Itoa(fileId) + ".log"
			filePath := filepath.Join(path, fileName)

			lf, err := OpenLogFile(filePath)
			if err != nil {
				return nil, err
			}

			db.files[fileId] = lf
			maxId = max(maxId, fileId)

			if err := db.readfile(lf, fileId, 0); err != nil {
				return nil, err
			}
		}
		db.activeId = maxId
		db.nextFileId = maxId + 1
	}

	return db, nil
}

func (db *DB) Put(key, val []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.put(key, val)

}

func (db *DB) put(key, val []byte) error {
	if err := db.rotateIfNeeded(); err != nil {
		return err
	}
	buffer := encode(key, val)
	activeFile := db.files[db.activeId]
	offset, err := activeFile.Write(buffer)

	if err != nil {
		return err
	}

	if len(val) == 0 {
		db.index.Delete(key)
	} else {
		db.index.Put(key, IndexEntry{Offset: offset, Size: int64(len(buffer)), FileId: db.activeId})
	}
	return nil
}

func (db *DB) Get(key []byte) (val []byte, err error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	entry, ok := db.index.Get(key)
	if !ok {
		return nil, errors.New("key not found")
	}

	file, ok := db.files[entry.FileId]
	if !ok {
		return nil, errors.New("file not found for entry")
	}
	buffer, err := file.ReadAt(entry.Offset, entry.Size)

	if err != nil {
		return nil, err
	}

	_, val, _, err = decode(buffer)
	return val, err

}

func (db *DB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.put(key, nil)
}

func (db *DB) readfile(lf *logfile, fileId int, offset int64) error {

	for {
		header, err := lf.ReadAt(offset, int64(headerSize))

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		keySize := binary.BigEndian.Uint32(header[8:12])
		valSize := binary.BigEndian.Uint32(header[12:16])
		recordLen := int64(headerSize) + int64(keySize) + int64(valSize)
		record, err := lf.ReadAt(offset, recordLen)

		if err != nil {
			return err
		}

		key, val, _, err := decode(record)

		if err != nil {
			return err
		}

		if len(val) == 0 {
			db.index.Delete(key)
		} else {
			db.index.Put(key, IndexEntry{Offset: offset, Size: recordLen, FileId: fileId})
		}

		offset += recordLen
	}

}

func (db *DB) rotateIfNeeded() error {
	activeFile := db.files[db.activeId]
	fileSize, err := activeFile.Size()
	if err != nil {
		return err
	}

	if fileSize > maxFileSize {
		newID := db.nextFileId
		db.nextFileId++

		fileName := strconv.Itoa(newID) + ".log"
		fullPath := filepath.Join(db.dir, fileName)
		lf, err := OpenLogFile(fullPath)
		if err != nil {
			return err
		}

		db.files[newID] = lf
		db.activeId = newID
	}

	return nil
}

func discoverFileIDs(dir string) ([]int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []int

	for i := range len(entries) {
		name := entries[i].Name()
		trimmed := strings.TrimSuffix(name, ".log")
		if name != trimmed {
			filename, err := strconv.Atoi(trimmed)
			if err != nil {
				return nil, err
			}
			files = append(files, filename)
		}
	}

	return files, nil
}

func (db *DB) isLive(key []byte, fileID int, offset int64) bool {
	entry, ok := db.index.Get(key)
	if !ok {
		return false
	}
	return entry.FileId == fileID && entry.Offset == offset
}

func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var eligible []int
	for id := range db.files {
		if id != db.activeId {
			eligible = append(eligible, id)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	newID := db.nextFileId
	db.nextFileId++

	newFileName := strconv.Itoa(newID) + ".log"
	newFilePath := filepath.Join(db.dir, newFileName)
	newFile, err := OpenLogFile(newFilePath)
	if err != nil {
		return err
	}

	for _, id := range eligible {
		lf := db.files[id]
		var offset int64 = 0

		for {
			header, err := lf.ReadAt(offset, int64(headerSize))

			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			keySize := binary.BigEndian.Uint32(header[8:12])
			valSize := binary.BigEndian.Uint32(header[12:16])
			recordLen := int64(headerSize) + int64(keySize) + int64(valSize)
			record, err := lf.ReadAt(offset, recordLen)

			if err != nil {
				return err
			}

			key, _, _, err := decode(record)

			if err != nil {
				return err
			}

			isLive := db.isLive(key, id, offset)

			if isLive {
				newOffset, err := newFile.Write(record)
				if err != nil {
					return err
				}
				db.index.Put(key, IndexEntry{FileId: newID, Offset: newOffset, Size: recordLen})
			}

			offset += recordLen
		}

	}

	for _, id := range eligible {
		lf := db.files[id]
		lf.Close()
		os.Remove(filepath.Join(db.dir, strconv.Itoa(id)+".log"))
		delete(db.files, id)
	}

	db.files[newID] = newFile

	return nil

}

func (db *DB) FileCount() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.files)
}

func (db *DB) ActiveFileID() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.activeId
}
