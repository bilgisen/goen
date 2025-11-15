package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bilgisen/goen/internal/config"
	"github.com/bilgisen/goen/internal/storage"
)

func main() {
	cfg := config.Load()
	if cfg.R2Endpoint == "" || cfg.R2AccessKey == "" || cfg.R2SecretKey == "" || cfg.R2Bucket == "" {
		log.Fatalf("R2 configuration is missing. Please set R2_ENDPOINT, R2_ACCESS_KEY, R2_SECRET_ACCESS_KEY, and R2_BUCKET")
	}

	r2, err := storage.NewR2Storage(cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket, cfg.R2AccountID)
	if err != nil {
		log.Fatalf("failed to initialize R2 storage: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Purging all objects under processed/ ...")
	if err := r2.PurgeProcessed(ctx); err != nil {
		log.Fatalf("purge failed: %v", err)
	}

	fmt.Println("Purge completed successfully.")
}
