package naive

import (
	"encoding/json"
	"os"
	"sync"
)

// NaiveStore represents the "obvious first attempt" at a persistent
// key-value store: keep everything in memory, and on every write,
// serialize the ENTIRE dataset back to disk from scratch.
type NaiveStore struct {
	mu   sync.Mutex
	path string
	data map[string]string
}

func Open(path string) (*NaiveStore, error) {
	data := make(map[string]string)

	if bytes, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(bytes, &data) // best-effort load of existing data
	}

	return &NaiveStore{path: path, data: data}, nil
}

func (s *NaiveStore) Put(key, val string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = val

	bytes, err := json.Marshal(s.data)
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, bytes, 0644)
}

func (s *NaiveStore) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, ok := s.data[key]
	return val, ok
}
