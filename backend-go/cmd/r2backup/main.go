// Command r2backup pushes a backup stream to R2 and prunes old ones.
//
// Offsite is the whole point: the nightly pg_dump written next to the
// database it came from protects against operator error and nothing else. A disk
// failure, a bad `docker compose down -v`, or losing the box takes the dump with
// it. This puts the same bytes somewhere that fails independently.
//
// Reads the payload on stdin so the caller decides what a backup is — a pg_dump
// pipe, a tar of uploads/ — without this binary needing pg_dump or knowing the
// layout. Retention is by key name, which carries the timestamp.
//
//	… | r2backup -key backups/db/anime_kage_20260727T0330Z.sql.gz -keep 30
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"time"

	"animekage/backend/internal/config"
	"animekage/backend/internal/storage"
)

func main() {
	key := flag.String("key", "", "destination object key (required)")
	keep := flag.Int("keep", 30, "how many objects to retain under the key's prefix")
	flag.Parse()

	if *key == "" {
		log.Fatal("-key is required")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	store, err := storage.New(cfg)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	if store == nil {
		log.Fatal("R2 is not configured (R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY / R2_BUCKET)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Spooled to a temp file rather than handed the pipe directly.
	//
	// The S3 SDK has to seek the body to sign the payload, and a pipe cannot
	// seek — passing os.Stdin fails with "seek /dev/stdin: illegal seek" *after*
	// the local backup has already been written, which reads as "the backup
	// worked but the offsite copy silently didn't". A temp file is seekable and,
	// unlike buffering in memory, stays correct when the database eventually is
	// not 35 KB.
	spool, err := os.CreateTemp("", "ak-backup-*")
	if err != nil {
		log.Fatalf("spool: %v", err)
	}
	defer os.Remove(spool.Name())
	defer spool.Close()

	n, err := io.Copy(spool, os.Stdin)
	if err != nil {
		log.Fatalf("read payload: %v", err)
	}
	if n == 0 {
		// An empty upload would happily replace a good backup with nothing.
		log.Fatal("refusing to upload an empty backup")
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		log.Fatalf("rewind spool: %v", err)
	}

	if err := store.PutStream(ctx, *key, "application/octet-stream", spool); err != nil {
		log.Fatalf("upload: %v", err)
	}
	fmt.Printf("uploaded %s (%d bytes)\n", *key, n)

	// Retention. Keys are timestamped and sorted lexically, so "oldest" is just
	// "first" — no metadata lookups, and it stays correct if a clock skews.
	prefix := path.Dir(*key) + "/"
	keys, err := store.List(ctx, prefix)
	if err != nil {
		log.Printf("warning: could not list %s for retention: %v", prefix, err)
		return
	}
	for i := 0; i < len(keys)-*keep; i++ {
		if err := store.Delete(ctx, keys[i]); err != nil {
			log.Printf("warning: could not prune %s: %v", keys[i], err)
			continue
		}
		fmt.Printf("pruned %s\n", keys[i])
	}
}
