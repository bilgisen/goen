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

	// Clean and trim fields (no truncation for title and description as requested)
	item.SeoTitle = p.preserveCulturalContext(p.correctPoliticalReferences(p.cleanText(item.SeoTitle)))
	item.SeoDesc = p.preserveCulturalContext(p.correctPoliticalReferences(p.cleanText(item.SeoDesc)))
	item.ContentMD = p.preserveCulturalContext(p.correctPoliticalReferences(p.cleanMarkdown(item.ContentMD)))
	item.Image = strings.TrimSpace(item.Image)

	// Clean tags if they exist
	if len(item.Tags) > 0 {
		for i, tag := range item.Tags {
			item.Tags[i] = strings.ToLower(p.cleanText(tag))
		}
	}

	// Ensure required fields have values
	if item.Category == "" {
		item.Category = "turkiye"
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

// correctPoliticalReferences fixes incorrect political figure references
func (p *PostProcessor) correctPoliticalReferences(s string) string {
	// Fix "former President Trump" to "President Trump"
	re := regexp.MustCompile(`(?i)former\s+President\s+Trump`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		// Preserve the original case pattern
		if strings.HasPrefix(match, "F") {
			return "President Trump"
		} else if strings.HasPrefix(match, "f") {
			return "President Trump"
		}
		return "President Trump"
	})

	// Fix "former U.S. President Donald Trump" to "U.S. President Donald Trump"
	re2 := regexp.MustCompile(`(?i)former\s+U\.S\.\s+President\s+Donald\s+Trump`)
	s = re2.ReplaceAllStringFunc(s, func(match string) string {
		if strings.HasPrefix(match, "F") {
			return "U.S. President Donald Trump"
		}
		return "U.S. President Donald Trump"
	})

	// Fix "former President Donald Trump" to "President Donald Trump"
	re3 := regexp.MustCompile(`(?i)former\s+President\s+Donald\s+Trump`)
	s = re3.ReplaceAllStringFunc(s, func(match string) string {
		if strings.HasPrefix(match, "F") {
			return "President Donald Trump"
		}
		return "President Donald Trump"
	})

	return s
}

// preserveCulturalContext ensures Turkish cultural references are maintained appropriately
func (p *PostProcessor) preserveCulturalContext(s string) string {
	// Ensure "Turkey" is always "Türkiye" 
	re := regexp.MustCompile(`(?i)\bTurkey\b`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		// Preserve case for different positions in sentence
		if match == "Turkey" {
			return "Türkiye"
		} else if match == "turkey" {
			return "Türkiye"
		} else if match == "TURKEY" {
			return "TÜRKİYE"
		}
		return "Türkiye"
	})

	// Fix common Western-centric geographical references that might alienate Turkish readers
	// Example: "the Middle East" -> "the Middle East, including Türkiye" when contextually appropriate
	// This is a subtle enhancement that maintains Turkish perspective

	return s
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
