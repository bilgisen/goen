package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bilgisen/goen/internal/models"
	"github.com/go-resty/resty/v2"
)

type Fetcher struct {
	client *resty.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: resty.New().
			SetTimeout(30 * time.Second).
			SetRetryCount(3).
			SetRetryWaitTime(2 * time.Second).
			SetRetryMaxWaitTime(10 * time.Second),
	}
}

// FetchFeed retrieves a feed from the given URL and parses it into FeedItems
func (f *Fetcher) FetchFeed(ctx context.Context, url string) ([]models.FeedItem, error) {
	var items []models.FeedItem

	resp, err := f.client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed from %s: %w", url, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode(), url)
	}

	// Try to parse the response as a single item or an array of items
	if err := json.Unmarshal(resp.Body(), &items); err != nil {
		// If it's not an array, try to parse as a single item
		var singleItem models.FeedItem
		if singleErr := json.Unmarshal(resp.Body(), &singleItem); singleErr != nil {
			return nil, fmt.Errorf("failed to parse feed response: %w (tried both array and single item)", err)
		}
		items = []models.FeedItem{singleItem}
	}

	return items, nil
}

// FetchMultipleFeeds concurrently fetches multiple feeds
func (f *Fetcher) FetchMultipleFeeds(ctx context.Context, urls []string) ([]models.FeedItem, error) {
	type result struct {
		items []models.FeedItem
		err   error
	}

	results := make(chan result, len(urls))

	for _, url := range urls {
		go func(u string) {
			items, err := f.FetchFeed(ctx, u)
			results <- result{items: items, err: err}
		}(url)
	}

	var allItems []models.FeedItem
	var errs []error

	for i := 0; i < len(urls); i++ {
		res := <-results
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		allItems = append(allItems, res.items...)
	}

	if len(errs) > 0 {
		return allItems, fmt.Errorf("encountered %d errors while fetching feeds, first error: %v", len(errs), errs[0])
	}

	return allItems, nil
}
