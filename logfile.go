package bitcask

import (
	"os"
)

type logfile struct {
	file *os.File
}

func OpenLogFile(path string) (*logfile, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &logfile{file: file}, nil
}

func (l *logfile) Write(record []byte) (offset int64, err error) {
	info, err := l.file.Stat()
	if err != nil {
		return 0, err
	}
	offset = info.Size()
	_, err = l.file.Write(record)
	if err != nil {
		return 0, err
	}

	return offset, nil
}

func (l *logfile) ReadAt(offset int64, size int64) ([]byte, error) {
	buf := make([]byte, size)
	_, err := l.file.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (l *logfile) Size() (int64, error) {
	info, err := l.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (l *logfile) Close() error {
	return l.file.Close()
}
