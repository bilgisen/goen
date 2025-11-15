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
	days := flag.Int("days", 7, "expiration days for processed/ objects")
	flag.Parse()

	cfg := config.Load()
	if cfg.R2Endpoint == "" || cfg.R2AccessKey == "" || cfg.R2SecretKey == "" || cfg.R2Bucket == "" {
		log.Fatalf("R2 configuration is missing. Please set R2_ENDPOINT, R2_ACCESS_KEY, R2_SECRET_ACCESS_KEY, and R2_BUCKET")
	}

	r2, err := storage.NewR2Storage(cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket, cfg.R2AccountID)
	if err != nil {
		log.Fatalf("failed to initialize R2 storage: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Printf("Setting lifecycle expiration for processed/ to %d days...\n", *days)
	if err := r2.EnsureProcessedExpiry(ctx, int32(*days)); err != nil {
		log.Fatalf("failed to set lifecycle: %v", err)
	}
	fmt.Println("Lifecycle configured successfully.")
}
