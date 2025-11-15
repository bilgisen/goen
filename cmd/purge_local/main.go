package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/bilgisen/goen/internal/config"
	"github.com/bilgisen/goen/internal/storage"
)

func main() {
	var timeoutSec int
	flag.IntVar(&timeoutSec, "timeout", 60, "timeout seconds")
	flag.Parse()

	cfg := config.Load()

	fs, err := storage.NewFileStorage(cfg.ProcessedPath)
	if err != nil {
		log.Fatalf("failed to init file storage: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	fmt.Println("Purging local processed JSON files ...")
	if err := fs.PurgeProcessedLocal(ctx); err != nil {
		log.Fatalf("local purge failed: %v", err)
	}
	fmt.Println("Local purge completed.")
}
