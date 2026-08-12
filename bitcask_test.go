package bitcask

import (
	"fmt"
	"testing"
)

func TestPutGet(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := db.Put([]byte("hello"), []byte("world")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	val, err := db.Get([]byte("hello"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "world" {
		t.Errorf("got %q, want %q", val, "world")
	}
}

func TestGetMissingKey(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_, err = db.Get([]byte("hello"))
	if err == nil {
		t.Fatal("expected an error for missing key, got nil")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := db.Put([]byte("hello"), []byte("world")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	val, err := db.Get([]byte("hello"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(val) != "world" {
		t.Errorf("got %q, want %q", val, "world")
	}

	db.Delete([]byte("hello"))

	val, err = db.Get([]byte("hello"))
	if err == nil {
		t.Fatalf("expected key not found after delete, got %v", err)
	}
}

func TestRecovery(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	db.Put([]byte("alpha"), []byte("one"))
	db.Put([]byte("beta"), []byte("two"))
	db.Delete([]byte("beta"))

	// Simulate a restart: open a fresh DB instance on the same directory.
	db2, err := Open(dir)
	if err != nil {
		t.Fatalf("re-Open failed: %v", err)
	}

	val, err := db2.Get([]byte("alpha"))
	if err != nil || string(val) != "one" {
		t.Errorf("alpha: got (%q, %v), want (\"one\", nil)", val, err)
	}

	_, err = db2.Get([]byte("beta"))
	if err == nil {
		t.Error("expected beta to be deleted, but Get succeeded")
	}
}

func TestRotation(t *testing.T) {
	originalMax := maxFileSize
	maxFileSize = 200
	defer func() { maxFileSize = originalMax }()

	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for round := 0; round < 15; round++ {
		db.Put([]byte("alpha"), []byte(fmt.Sprintf("alpha-v%d", round)))
		db.Put([]byte("beta"), []byte(fmt.Sprintf("beta-v%d", round)))
		db.Put([]byte("gamma"), []byte(fmt.Sprintf("gamma-v%d", round)))
	}

	if db.FileCount() <= 1 {
		t.Fatalf("expected rotation to occur, but only %d file(s) tracked", db.FileCount())
	}

	val, err := db.Get([]byte("alpha"))
	if err != nil || string(val) != "alpha-v14" {
		t.Errorf("alpha: got (%q, %v), want (\"alpha-v14\", nil)", val, err)
	}

	val, err = db.Get([]byte("gamma"))
	if err != nil || string(val) != "gamma-v14" {
		t.Errorf("gamma: got (%q, %v), want (\"gamma-v14\", nil)", val, err)
	}

}

func TestCompact(t *testing.T) {
	originalMax := maxFileSize
	maxFileSize = 200
	defer func() { maxFileSize = originalMax }()

	dir := t.TempDir()

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for round := 0; round < 15; round++ {
		db.Put([]byte("alpha"), []byte(fmt.Sprintf("alpha-v%d", round)))
		db.Put([]byte("beta"), []byte(fmt.Sprintf("beta-v%d", round)))
		db.Put([]byte("gamma"), []byte(fmt.Sprintf("gamma-v%d", round)))
	}

	initialFileCount := db.FileCount()

	if initialFileCount <= 1 {
		t.Fatalf("expected rotation to occur, but only %d file(s) tracked", db.FileCount())
	}

	db.Delete([]byte("beta"))

	if err := db.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if db.FileCount() >= initialFileCount {
		t.Fatalf("expected compaction to reduce file count, got %d (was %d)", db.FileCount(), initialFileCount)
	}

	val, err := db.Get([]byte("alpha"))
	if err != nil || string(val) != "alpha-v14" {
		t.Errorf("alpha: got (%q, %v), want (\"alpha-v14\", nil)", val, err)
	}

	val, err = db.Get([]byte("gamma"))
	if err != nil || string(val) != "gamma-v14" {
		t.Errorf("gamma: got (%q, %v), want (\"gamma-v14\", nil)", val, err)
	}

	_, err = db.Get([]byte("beta"))
	if err == nil {
		t.Error("expected beta to be deleted, but Get succeeded")
	}
}

func BenchmarkPut(b *testing.B) {
	dir := b.TempDir()
	db, err := Open(dir)
	if err != nil {
		b.Fatalf("Open failed: %v", err)
	}

	key := []byte("benchkey")
	val := []byte("some-benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.Put(key, val)
	}
}
