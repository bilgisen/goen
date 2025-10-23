package feed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bilgisen/goen/internal/cache"
	"github.com/bilgisen/goen/internal/logger"
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
	log := logger.Get()
	start := time.Now()
	log.Info().
		Strs("feed_urls", feedURLs).
		Msg("Starting to process feeds")

	// Fetch all feeds concurrently
	items, err := p.fetcher.FetchMultipleFeeds(ctx, feedURLs)
	if err != nil {
		log.Error().
			Err(err).
			Msg("Error fetching feeds")
		return nil, fmt.Errorf("error fetching feeds: %w", err)
	}

	log.Info().
		Int("total_items", len(items)).
		Dur("fetch_duration", time.Since(start)).
		Msg("Fetched feed items")

	// Process and validate feed items
	validItems, errs := p.parser.ProcessFeedItems(ctx, items)
	if len(errs) > 0 {
		log.Warn().
			Errs("validation_errors", errs).
			Msg("Encountered validation errors while processing feed items")
	}

	log.Info().
		Int("valid_items", len(validItems)).
		Dur("validation_duration", time.Since(start)).
		Msg("Validated feed items")

	// Filter out duplicates using cache
	uniqueItems, err := p.filterDuplicates(ctx, validItems)
	if err != nil {
		log.Error().
			Err(err).
			Msg("Error filtering duplicates")
		return nil, fmt.Errorf("error filtering duplicates: %w", err)
	}

	log.Info().
		Int("unique_items", len(uniqueItems)).
		Dur("total_duration", time.Since(start)).
		Msg("Finished processing feeds")

	return uniqueItems, nil
}

// filterDuplicates removes items that have already been processed
func (p *Processor) filterDuplicates(ctx context.Context, items []models.FeedItem) ([]models.FeedItem, error) {
	log := logger.Get()
	start := time.Now()
	log.Info().
		Int("total_items", len(items)).
		Msg("Starting to filter duplicates")

	if len(items) == 0 {
		log.Warn().Msg("No items to filter - empty input slice")
		return []models.FeedItem{}, nil
	}

	var uniqueItems []models.FeedItem
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Use a semaphore to limit concurrent cache checks
	semaphore := make(chan struct{}, 10)
	errChan := make(chan error, 1)
	processedCount := 0
	duplicateCount := 0

	for _, item := range items {
		select {
		case <-ctx.Done():
			log.Warn().
				Int("processed_items", processedCount).
				Int("unique_items", len(uniqueItems)).
				Msg("Context cancelled while filtering duplicates")
			return nil, ctx.Err()
		case semaphore <- struct{}{}:
		}

		item := item // Create a new variable for the goroutine

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()

			if item.Url == "" {
				log.Warn().
					Str("guid", item.Guid).
					Str("title", item.TitleTR).
					Msg("Skipping item with empty URL")
				return
			}

			hash := utils.Hash(item.Url)
			isProcessed, err := p.cache.IsProcessed(ctx, hash)
			if err != nil {
				log.Error().
					Err(err).
					Str("url", item.Url).
					Str("guid", item.Guid).
					Msg("Error checking cache for item")
				return
			}

			if isProcessed {
				log.Debug().
					Str("url", item.Url).
					Str("guid", item.Guid).
					Msg("Skipping already processed item")
				mu.Lock()
				duplicateCount++
				mu.Unlock()
			} else {
				log.Debug().
					Str("url", item.Url).
					Str("guid", item.Guid).
					Str("title", item.TitleTR).
					Msg("Adding new unique item")
				mu.Lock()
				uniqueItems = append(uniqueItems, item)
				mu.Unlock()
			}

			// Update the processed count in a thread-safe way
			mu.Lock()
			processedCount++
			if processedCount%10 == 0 { // Log progress every 10 items
				log.Info().
					Int("processed", processedCount).
					Int("unique_so_far", len(uniqueItems)).
					Int("duplicates_so_far", duplicateCount).
					Msg("Filtering progress")
			}
			mu.Unlock()
		}()
	}

	// Wait for all goroutines to complete or an error to occur
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info().
			Int("total_processed", len(items)).
			Int("unique_items", len(uniqueItems)).
			Int("duplicate_items", duplicateCount).
			Dur("duration", time.Since(start)).
			Msg("Finished filtering duplicates")
		return uniqueItems, nil
	case err := <-errChan:
		return nil, err
	}
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
