package feed

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bilgisen/goen/internal/cache"
	"github.com/bilgisen/goen/internal/models"
	"github.com/bilgisen/goen/internal/utils"
)

type Processor struct {
	fetcher *Fetcher
	parser  *Parser
	cache   *cache.RedisClient
}

func NewProcessor(redisClient *cache.RedisClient) *Processor {
	return &Processor{
		fetcher: NewFetcher(),
		parser:  NewParser(),
		cache:   redisClient,
	}
}

// ProcessFeeds fetches, parses, and processes feeds from the given URLs
func (p *Processor) ProcessFeeds(ctx context.Context, feedURLs []string) ([]models.FeedItem, error) {
	// Fetch all feeds concurrently
	items, err := p.fetcher.FetchMultipleFeeds(ctx, feedURLs)
	if err != nil {
		return nil, fmt.Errorf("error fetching feeds: %w", err)
	}

	// Process and validate feed items
	validItems, errs := p.parser.ProcessFeedItems(ctx, items)
	if len(errs) > 0 {
		// Log validation errors but continue processing
		for _, e := range errs {
			log.Printf("Validation error: %v", e)
		}
	}

	// Filter out duplicates using cache
	uniqueItems, err := p.filterDuplicates(ctx, validItems)
	if err != nil {
		return nil, fmt.Errorf("error filtering duplicates: %w", err)
	}

	return uniqueItems, nil
}

// filterDuplicates removes items that have already been processed
func (p *Processor) filterDuplicates(ctx context.Context, items []models.FeedItem) ([]models.FeedItem, error) {
	var uniqueItems []models.FeedItem
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 10) // Limit concurrent cache checks

	for _, item := range items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case semaphore <- struct{}{}:
		}

		item := item // Create a new variable for the goroutine

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()

			hash := utils.Hash(item.Url)
			processed, err := p.cache.IsProcessed(ctx, hash)
			if err != nil {
				log.Printf("Error checking cache for item %s: %v", item.Url, err)
				return
			}

			if !processed {
				mu.Lock()
				uniqueItems = append(uniqueItems, item)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return uniqueItems, nil
}

// MarkAsProcessed marks the given URLs as processed in the cache
func (p *Processor) MarkAsProcessed(ctx context.Context, urls []string, ttl time.Duration) error {
	for _, url := range urls {
		hash := utils.Hash(url)
		if err := p.cache.MarkProcessed(ctx, hash, ttl); err != nil {
			return fmt.Errorf("error marking %s as processed: %w", url, err)
		}
	}
	return nil
}
