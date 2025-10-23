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

// JSONFeed represents the structure of the JSON feed from the URL
type JSONFeed struct {
	FeedLink     string `json:"feed_link"`
	FeedTitle    string `json:"feed_title"`
	Items        []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Guid        string `json:"guid"`
		Published   string `json:"published"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Image       string `json:"image"`
	} `json:"items"`
	ItemsReturned int `json:"items_returned"`
	ItemsSkipped  int `json:"items_skipped"`
}

// FetchFeed retrieves a feed from the given URL and parses it into FeedItems
func (f *Fetcher) FetchFeed(ctx context.Context, url string) ([]models.FeedItem, error) {
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

	// Try to parse as JSON feed structure first
	var jsonFeed JSONFeed
	if err := json.Unmarshal(resp.Body(), &jsonFeed); err == nil && len(jsonFeed.Items) > 0 {
		// Successfully parsed as JSON feed, convert to our model
		items := make([]models.FeedItem, 0, len(jsonFeed.Items))
		for _, item := range jsonFeed.Items {
			// Use link as fallback if guid is empty
			guid := item.Guid
			if guid == "" {
				guid = item.Link
			}

			items = append(items, models.FeedItem{
				Guid:      guid,
				TitleTR:   item.Title,
				ContentTR: item.Content,
				Image:     item.Image,
				Url:       item.Link,
				Category:  "general", // Default category
			})
		}
		return items, nil
	}

	// Fallback to the original parsing logic for other formats
	var items []models.FeedItem
	if err := json.Unmarshal(resp.Body(), &items); err != nil {
		// If it's not an array, try to parse as a single item
		var singleItem models.FeedItem
		if singleErr := json.Unmarshal(resp.Body(), &singleItem); singleErr != nil {
			return nil, fmt.Errorf("failed to parse feed response: %w (tried both JSON feed and array formats)", err)
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
