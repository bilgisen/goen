package ai

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bilgisen/goen/internal/models"
)

type PostProcessor struct {
	maxTitleLength       int
	maxDescriptionLength int
	minContentLength     int
}

func NewPostProcessor() *PostProcessor {
	return &PostProcessor{
		maxTitleLength:       60,
		maxDescriptionLength: 160,
		minContentLength:     50,
	}
}

// ProcessNewsItem validates and cleans the AI-generated news item
func (p *PostProcessor) ProcessNewsItem(item *models.NewsItem) error {
	// Validate required fields
	if item.SeoTitle == "" {
		return fmt.Errorf("missing required field: seo_title")
	}
	if item.SeoDesc == "" {
		return fmt.Errorf("missing required field: seo_description")
	}
	if len(item.ContentMD) < p.minContentLength {
		return fmt.Errorf("content too short, minimum %d characters required", p.minContentLength)
	}

	// Clean and trim fields
	item.SeoTitle = p.cleanText(item.SeoTitle)
	item.SeoDesc = p.cleanText(item.SeoDesc)
	item.ContentMD = p.cleanMarkdown(item.ContentMD)

	// Truncate if necessary
	if len(item.SeoTitle) > p.maxTitleLength {
		item.SeoTitle = item.SeoTitle[:p.maxTitleLength-3] + "..."
	}
	if len(item.SeoDesc) > p.maxDescriptionLength {
		item.SeoDesc = item.SeoDesc[:p.maxDescriptionLength-3] + "..."
	}

	// Ensure required fields have values
	if item.Category == "" {
		item.Category = "General"
	}
	if len(item.Tags) == 0 {
		item.Tags = []string{"news", item.Category}
	}

	// Set timestamps
	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	return nil
}

// cleanText removes unwanted characters and normalizes whitespace
func (p *PostProcessor) cleanText(s string) string {
	// Remove control characters
	re := regexp.MustCompile(`[\x00-\x1F\x7F]`)
	s = re.ReplaceAllString(s, " ")

	// Normalize whitespace
	s = strings.Join(strings.Fields(s), " ")

	return strings.TrimSpace(s)
}

// cleanMarkdown cleans and validates markdown content
func (p *PostProcessor) cleanMarkdown(content string) string {
	// Remove any potential XSS - using a simpler regex that doesn't use negative lookahead
	re := regexp.MustCompile(`<script[^>]*>[\s\S]*?<\/script>`)
	content = re.ReplaceAllString(content, "")

	// Also remove other potentially dangerous HTML tags
	dangerousTags := []string{"<script", "<iframe", "<object", "<embed", "<link", "<meta"}
	for _, tag := range dangerousTags {
		re := regexp.MustCompile(fmt.Sprintf(`<%s[^>]*>`, tag))
		content = re.ReplaceAllString(content, "")
	}

	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Ensure proper markdown formatting
	content = p.ensureMarkdownFormatting(content)

	return content
}

// ensureMarkdownFormatting applies basic markdown formatting if missing
func (p *PostProcessor) ensureMarkdownFormatting(content string) string {
	// If the content doesn't look like markdown, wrap it in paragraphs
	if !strings.Contains(content, "\n\n") && !strings.Contains(content, "# ") {
		return fmt.Sprintf("\n\n%s\n\n", content)
	}
	return content
}

// ProcessBatch processes multiple news items in parallel
func (p *PostProcessor) ProcessBatch(items []*models.NewsItem) ([]*models.NewsItem, []error) {
	type result struct {
		item *models.NewsItem
		err  error
	}

	results := make(chan result, len(items))

	for _, item := range items {
		go func(i *models.NewsItem) {
			err := p.ProcessNewsItem(i)
			results <- result{item: i, err: err}
		}(item)
	}

	var validItems []*models.NewsItem
	var errors []error

	for i := 0; i < len(items); i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, fmt.Errorf("error processing item %s: %w", res.item.ID, res.err))
			continue
		}
		validItems = append(validItems, res.item)
	}

	return validItems, errors
}
