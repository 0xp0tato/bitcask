# bitcask

A small, from-scratch implementation of [Bitcask](https://riak.com/assets/bitcask-intro.pdf) — the log-structured, append-only key-value storage engine originally built for Riak — written in Go.

Built to get hands-on with storage engine internals: append-only writes, crash recovery, file rotation, and compaction.

## Why Bitcask

Bitcask trades disk space for extremely simple, extremely fast writes:

- **Writes are always appends.** Nothing is ever overwritten in place, which makes writes sequential (fast) and crash-safe (a crash can only corrupt the very last, in-progress record — everything before it is untouched).
- **Reads go through an in-memory index.** A `map[key] -> (file, offset, size)` means a `Get` is one map lookup plus one seek-and-read — no scanning.
- **The index is disposable.** It lives entirely in memory and is rebuilt from the log on every startup, which is what makes crash recovery simple: the log is the single source of truth, the index is just a cache over it.

## Design

### Record format

Every write — a `Put` or a `Delete` — is encoded as a single binary record and appended to the active log file:

```
+----------+-----------+------------+-------------+------+-------+
| crc (4B) | tstamp(4B)| keySize(4B)| valSize(4B) | key  | value |
+----------+-----------+------------+-------------+------+-------+
```

- A fixed 16-byte header (`crc`, `timestamp`, `keySize`, `valSize`), followed by the variable-length key and value.
- `crc` is a CRC32 checksum over everything after itself. On recovery, a checksum mismatch means the record is the corrupted tail of an interrupted write — recovery stops there rather than trusting garbage data.
- **Deletes are tombstones**, not a separate operation: `Delete(key)` writes a normal record with `valSize == 0`. Anything reading the log — live writes or recovery — treats a zero-length value as "this key is gone."

### In-memory index

```go
type IndexEntry struct {
    FileID int
    Offset int64
    Size   int64
}
```

A `map[string]IndexEntry` guarded by a `sync.RWMutex`. `Put`/`Delete` take the write lock; `Get` takes a read lock, so multiple reads can proceed concurrently but never overlap with a write.

### Put / Get / Delete

- **Put**: encode the record → append to the active file → update the index to point at the new location.
- **Get**: look up the index → seek to `(FileID, Offset)` → read `Size` bytes → decode → return the value. A missing index entry returns an error rather than a zero value, so callers can distinguish "not found" from "found, empty."
- **Delete**: append a tombstone record (same code path as `Put`, with a `nil`/empty value) → remove the key from the index.

## Crash recovery

The index is never persisted — it's rebuilt from scratch every time the database is opened, by scanning every log file from byte 0 to EOF and replaying each record:

- A normal record updates the index to point at that `(FileID, Offset)`.
- A tombstone record removes the key from the index.

Because the log is append-only and each record is self-describing (the header tells you exactly how many more bytes to read), this scan is a straightforward sequential pass — no separate manifest or write-ahead log is needed.

This was verified with a real two-process test: one process writes and exits, a second, completely separate process opens the same directory and reads the data back correctly — proving recovery works from disk alone, with no in-memory state carried over.

## Concurrency

A single `sync.RWMutex` on `DB` guards both the index and file writes:

- `Put`/`Delete`/`Compact` take the exclusive write lock.
- `Get` takes a shared read lock, so concurrent reads don't block each other.

This is intentionally a single global lock rather than sharded/striped locking — simple and correct, with read/write throughput as the main trade-off under heavy concurrent write load (see Limitations).

## File rotation

Rather than one file growing forever, the active log file rotates once it exceeds a size threshold:

- Files are named `<id>.log`, where `id` is a simple incrementing integer — not a timestamp, to avoid any possibility of collision if multiple rotations happen within the same second.
- Only one file is "active" (currently being written to) at a time; `IndexEntry.FileID` records which file each key's current value actually lives in, so `Get` always reads from the right place regardless of how many rotations have happened since a key was last written.
- On reopening a directory with existing files, `Open` discovers all `*.log` files, opens and recovers each one, and resumes writing to the file with the highest ID.

## Compaction

Every overwrite or delete leaves the *old* record behind on disk — it's simply no longer pointed to by the index. Left unchecked, disk usage grows without bound even if the working set of keys stays small.

`Compact()`:

1. Selects every file **except** the currently active one (which may still be receiving writes).
2. Scans each eligible file record by record. For each record, checks whether the index's *current* entry for that key still points at this exact `(FileID, Offset)` — if so, the record is live; otherwise it's a stale, superseded copy and is dropped.
3. Copies every live record into one freshly created merge file, updating the index to point at the new location as it goes.
4. Closes and deletes the old files, and registers the merge file in their place.

The merge file is given a brand-new ID (from a monotonically increasing `nextFileId` counter, tracked separately from `activeId`) rather than reusing an old file's ID, which avoids any possibility of colliding with an ID `rotateIfNeeded` might independently hand out.

## Known limitations

Things a production-grade version would need that this one doesn't have:

- **No `fsync` policy.** Writes are handed to the OS via a normal buffered `Write`, but there's no explicit `fsync`/`Sync()` call, so a write isn't guaranteed durable against an OS crash or power loss until the OS itself flushes its buffers.
- **No hint files.** Real Bitcask writes a compact "hint file" alongside each data file during compaction, so recovery can rebuild the index from a small summary instead of re-scanning full data files. This implementation always does a full scan.
- **Single global lock.** One `RWMutex` for the whole database. Under heavy concurrent write load this serializes all writers; a production system might shard the index/lock by key range.
- **`maxFileSize` isn't configurable per database instance** — it's a package-level variable, primarily so tests can lower it to force rotation without writing megabytes of data. A real API would likely take this as an `Open` option.
- **All log files are held open for the lifetime of the process.** Fine for a small number of files; a long-running database with heavy rotation would eventually want to open files on demand and cache/evict handles instead.

## Benchmarks
 
```
BenchmarkPut-16    268700    4166 ns/op
```
 
~240,000 single-threaded writes/sec on an AMD Ryzen 7 5800H.
 
### Bitcask vs. the naive approach
 
To make the append-only design's payoff concrete, `naive/` implements the "obvious first attempt" at a persistent key-value store: keep everything in a map, and on every write, serialize the *entire* dataset back to disk from scratch (`json.Marshal` + `os.WriteFile`). Benchmarking both with a growing keyspace (2,000 unique keys):
 
| Approach | ns/op | Relative |
|---|---|---|
| Naive (rewrite whole file per write) | 634,732 | baseline |
| Bitcask (append-only) | 4,331 | **~146x faster** |
 
The gap isn't a fixed constant — it widens as the dataset grows. The naive approach's write cost is **O(n)** in the size of the existing dataset, since every write re-serializes everything that came before it; Bitcask's is **O(1)**, since a write is always just one append plus one index update, regardless of how much data already exists. This is the core justification for Bitcask's design, and the actual motivation behind log-structured storage engines generally.
 
Reproduce with:
```
go test ./naive/ -bench=BenchmarkNaivePut -run=^$ -benchtime=2000x
go test ./naive/ -bench=BenchmarkBitcaskPutGrowing -run=^$ -benchtime=2000x
```
 
## Usage
 
```go
db, err := bitcask.Open("data/")
if err != nil {
    log.Fatal(err)
}
 
err = db.Put([]byte("key"), []byte("value"))
val, err := db.Get([]byte("key"))
err = db.Delete([]byte("key"))
err = db.Compact()
```
 
## Running tests
 
```
go test ./...
go test -bench=. -run=^$
```
 
