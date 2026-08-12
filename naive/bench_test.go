package naive

import (
	"fmt"
	"testing"

	"bitcask"
)

func BenchmarkNaivePut(b *testing.B) {
	dir := b.TempDir()
	store, err := Open(dir + "/naive.json")
	if err != nil {
		b.Fatalf("open failed: %v", err)
	}

	val := "some-benchmark-value"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		store.Put(key, val)
	}
}

func BenchmarkBitcaskPutGrowing(b *testing.B) {
	dir := b.TempDir()
	db, err := bitcask.Open(dir)
	if err != nil {
		b.Fatalf("open failed: %v", err)
	}

	val := []byte("some-benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		db.Put(key, val)
	}
}
