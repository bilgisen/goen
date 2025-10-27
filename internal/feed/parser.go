package feed

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"

	"github.com/bilgisen/goen/internal/logger"
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
// extractCategory intelligently determines the category from URL, title, and content
func (p *Parser) extractCategory(url, title, content string) string {
	// Category keywords mapping
	categoryKeywords := map[string][]string{
		"politics":    {"politik", "siyasi", "hükümet", "bakan", "cumhurbaşkanı", "milletvekili", "seçim", "tbmm", "siyaset"},
		"economy":     {"ekonomi", "dolar", "euro", "faiz", "enflasyon", "büyüme", "gdp", "piyasa", "borsa", "hisse"},
		"technology":  {"teknoloji", "yazılım", "app", "uygulama", "ai", "yapay zeka", "robot", "dijital", "startup"},
		"sports":      {"spor", "futbol", "maç", "gol", "takım", "lig", "şampiyona", "olimpiyat", "atlet"},
		"health":      {"sağlık", "hastane", "doktor", "tedavi", "ilaç", "aşı", "pandemi", "covid", "hastalık"},
		"world":       {"dünya", "uluslararası", "abd", "amerika", "avrupa", "asya", "afrika", "bm", "nato"},
		"culture":     {"kültür", "sanat", "müzik", "film", "sinema", "tiyatro", "kitap", "edebiyat", "festival"},
		"education":   {"eğitim", "üniversite", "okul", "öğrenci", "öğretmen", "sınav", "mezun", "akademi"},
		"science":     {"bilim", "araştırma", "çalışma", "uzay", "teknoloji", "inovasyon", "keşif"},
	}

	// Clean and lowercase all inputs
	title = strings.ToLower(p.CleanHTML(title))
	content = strings.ToLower(p.CleanHTML(content))
	url = strings.ToLower(url)

	// Check URL for category indicators
	for category, keywords := range categoryKeywords {
		for _, keyword := range keywords {
			if strings.Contains(url, keyword) {
				return category
			}
		}
	}

	// Check title for category indicators
	for category, keywords := range categoryKeywords {
		for _, keyword := range keywords {
			if strings.Contains(title, keyword) {
				return category
			}
		}
	}

	// Check content for category indicators (less weight)
	for category, keywords := range categoryKeywords {
		for _, keyword := range keywords {
			if strings.Contains(content, keyword) {
				return category
			}
		}
	}

	// Default categories based on common Turkish news site patterns
	if strings.Contains(url, "siyaset") || strings.Contains(url, "politik") {
		return "politics"
	}
	if strings.Contains(url, "ekonomi") || strings.Contains(url, "finans") {
		return "economy"
	}
	if strings.Contains(url, "spor") {
		return "sports"
	}
	if strings.Contains(url, "teknoloji") || strings.Contains(url, "tech") {
		return "technology"
	}

	return "general"
}

// NormalizeFeedItem cleans and validates a single feed item
func (p *Parser) NormalizeFeedItem(item models.FeedItem) models.FeedItem {
	// Use smart category extraction
	smartCategory := p.extractCategory(item.Url, item.TitleTR, item.ContentTR)

	return models.FeedItem{
		Guid:      strings.TrimSpace(item.Guid),
		TitleTR:   p.CleanHTML(item.TitleTR),
		ContentTR: p.CleanHTML(item.ContentTR),
		Image:     strings.TrimSpace(item.Image),
		Url:       strings.TrimSpace(item.Url),
		Category:  smartCategory,
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
	log := logger.Get()
	log.Debug().
		Int("total_items", len(items)).
		Msg("Starting to process feed items")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var validItems []models.FeedItem
	var errors []error

	semaphore := make(chan struct{}, 10) // Limit concurrent processing

	for i, item := range items {
		select {
		case <-ctx.Done():
			log.Warn().
				Int("processed_items", i).
				Int("valid_items", len(validItems)).
				Msg("Context cancelled while processing feed items")
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
				log.Debug().
					Str("guid", item.Guid).
					Str("title", item.TitleTR).
					Str("url", item.Url).
					Err(err).
					Msg("Validation failed for feed item")
				mu.Lock()
				errors = append(errors, fmt.Errorf("invalid feed item %s: %w", item.Guid, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			validItems = append(validItems, normalized)
			mu.Unlock()

			log.Debug().
				Str("guid", item.Guid).
				Str("title", item.TitleTR).
				Msg("Successfully processed feed item")
		}()

		// Log progress every 10 items
		if i > 0 && i%10 == 0 {
			log.Debug().
				Int("processed", i).
				Int("valid_so_far", len(validItems)).
				Msg("Processing feed items")
		}
	}

	wg.Wait()

	log.Info().
		Int("total_processed", len(items)).
		Int("valid_items", len(validItems)).
		Int("validation_errors", len(errors)).
		Msg("Finished processing feed items")

	return validItems, errors
}
