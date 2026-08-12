package bitcask

type IndexEntry struct {
	Offset int64
	Size   int64
	FileId int
}

type Index struct {
	entries map[string]IndexEntry
}

func NewIndex() *Index {
	return &Index{
		entries: make(map[string]IndexEntry),
	}
}

func (idx *Index) Put(key []byte, entry IndexEntry) {
	idx.entries[string(key)] = entry
}

func (idx *Index) Get(key []byte) (IndexEntry, bool) {
	entry, ok := idx.entries[string(key)]
	return entry, ok
}

func (idx *Index) Delete(key []byte) {
	delete(idx.entries, string(key))
}
