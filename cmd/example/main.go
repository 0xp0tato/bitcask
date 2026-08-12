package main

import (
	"bitcask"
	"fmt"
)

func main() {
	db, err := bitcask.Open("logs")
	if err != nil {
		fmt.Println("open error:", err)
		return
	}

	// Overwrite the same 3 keys repeatedly — this creates lots of
	// dead records across multiple rotated files.
	for round := 0; round < 15; round++ {
		db.Put([]byte("alpha"), []byte(fmt.Sprintf("alpha-v%d", round)))
		db.Put([]byte("beta"), []byte(fmt.Sprintf("beta-v%d", round)))
		db.Put([]byte("gamma"), []byte(fmt.Sprintf("gamma-v%d", round)))
	}
	db.Delete([]byte("beta")) // beta should end up gone entirely

	fmt.Println("before compact — files tracked:", db.FileCount(), "activeId:", db.ActiveFileID())

	if err := db.Compact(); err != nil {
		fmt.Println("compact error:", err)
		return
	}

	fmt.Println("after compact — files tracked:", db.FileCount())

	val, err := db.Get([]byte("alpha"))
	fmt.Println("alpha:", string(val), err) // should be alpha-v14

	val, err = db.Get([]byte("beta"))
	fmt.Println("beta (deleted):", string(val), err) // should be not found

	val, err = db.Get([]byte("gamma"))
	fmt.Println("gamma:", string(val), err) // should be gamma-v14
}
