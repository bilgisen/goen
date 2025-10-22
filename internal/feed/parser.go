package feed

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"

	"github.com/bilgisen/goen/internal/models"
)

// Parser handles cleaning and normalizing feed items
type Parser struct {
	htmlTagRegex *regexp.Regexp
}

func NewParser() *Parser {
	return &Parser{
		htmlTagRegex: regexp.MustCompile(`<[^>]*>`),
	}
}

// CleanHTML removes HTML tags and normalizes whitespace
func (p *Parser) CleanHTML(input string) string {
	// Remove HTML tags
	cleaned := p.htmlTagRegex.ReplaceAllString(input, " ")
	// Unescape HTML entities
	cleaned = html.UnescapeString(cleaned)
	// Normalize whitespace
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

// NormalizeFeedItem cleans and validates a single feed item
func (p *Parser) NormalizeFeedItem(item models.FeedItem) models.FeedItem {
	return models.FeedItem{
		Guid:      strings.TrimSpace(item.Guid),
		TitleTR:   p.CleanHTML(item.TitleTR),
		ContentTR: p.CleanHTML(item.ContentTR),
		Image:     strings.TrimSpace(item.Image),
		Url:       strings.TrimSpace(item.Url),
		Category:  strings.TrimSpace(item.Category),
	}
}

// ValidateFeedItem checks if the feed item has the required fields
func (p *Parser) ValidateFeedItem(item models.FeedItem) error {
	if item.Guid == "" {
		return fmt.Errorf("missing required field: guid")
	}
	if item.TitleTR == "" {
		return fmt.Errorf("missing required field: title")
	}
	if item.Url == "" {
		return fmt.Errorf("missing required field: url")
	}
	return nil
}

// ProcessFeedItems concurrently processes a slice of feed items
func (p *Parser) ProcessFeedItems(ctx context.Context, items []models.FeedItem) ([]models.FeedItem, []error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var validItems []models.FeedItem
	var errors []error

	semaphore := make(chan struct{}, 10) // Limit concurrent processing

	for _, item := range items {
		select {
		case <-ctx.Done():
			return validItems, append(errors, ctx.Err())
		case semaphore <- struct{}{}:
		}

		item := item // Create a new variable for the goroutine

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()

			normalized := p.NormalizeFeedItem(item)
			if err := p.ValidateFeedItem(normalized); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("invalid feed item %s: %w", item.Guid, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			validItems = append(validItems, normalized)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return validItems, errors
}
