// internal/ai/gemini_client.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilgisen/goen/internal/logger"
	"github.com/bilgisen/goen/internal/models"
	"github.com/go-resty/resty/v2"
)

type GeminiClient struct {
	client    *resty.Client
	apiKey    string
	model     string
	baseURL   string
	limiter   *RedisLimiter // Redis-based rate limiter
	tpmLimit  int           // Tokens per minute limit
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// NewGeminiClient creates a new Gemini client with Redis-based rate limiting
// rpm is the maximum number of requests per minute
// tpm is the maximum number of tokens per minute
// redisURL is the connection string for Redis (e.g., "redis://user:password@localhost:6379/0")
func NewGeminiClient(apiKey, model string, rpm, tpm int, redisURL string) (*GeminiClient, error) {
	var limiter *RedisLimiter
	var err error

	// Initialize Redis limiter if URL is provided
	if redisURL != "" {
		limiter, err = NewRedisLimiter(redisURL, "gemini", rpm, tpm)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize rate limiter: %w", err)
		}
	}

	client := resty.New().
		SetTimeout(60 * time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(2 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second).
		AddRetryCondition(
			func(r *resty.Response, err error) bool {
				// Retry on 429 (Too Many Requests) or 5xx errors
				return r.StatusCode() == 429 || (r.StatusCode() >= 500 && r.StatusCode() < 600)
			},
		)

	return &GeminiClient{
		client:   client,
		apiKey:   apiKey,
		model:    model,
		baseURL:  "https://generativelanguage.googleapis.com/v1beta/models",
		limiter:  limiter,
		tpmLimit: tpm,
	}, nil
}

// GenerateEnglishNews processes a Turkish news item and returns an English version
func (g *GeminiClient) GenerateEnglishNews(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	// Apply rate limiting if enabled
	if g.limiter != nil {
		// Use the improved token estimation function
		estimatedTokens := estimateTokens(item.ContentTR)
		if err := g.limiter.WaitIfNeeded(ctx, estimatedTokens); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}
	}
	log := logger.Get()
	log.Info().
		Str("guid", item.Guid).
		Str("title", item.TitleTR).
		Msg("Starting to process news item")

	// Set a timeout context
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Retry up to 3 times with exponential backoff
	var newsItem *models.NewsItem
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			log.Debug().
				Str("guid", item.Guid).
				Int("attempt", attempt).
				Msg("Retrying news processing")
		}

		newsItem, lastErr = g.generateEnglishNewsOnce(ctx, item)
		if lastErr == nil {
			break
		}

		// Don't retry on validation errors, only on API errors
		if strings.Contains(lastErr.Error(), "missing required field") ||
		   strings.Contains(lastErr.Error(), "content too short") {
			break
		}

		// Wait before retrying (except on last attempt)
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
				// Continue to next attempt
			}
		}
	}

	if lastErr != nil {
		log.Error().
			Err(lastErr).
			Str("guid", item.Guid).
			Msg("Failed to generate English news after all retries")
		return nil, lastErr
	}

	log.Info().
		Str("guid", item.Guid).
		Str("title", newsItem.SeoTitle).
		Msg("Successfully processed news item")

	return newsItem, nil
}

func (g *GeminiClient) generateEnglishNewsOnce(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	// Build the prompt
	prompt := buildPrompt(item)
	log := logger.Get()
	log.Debug().
		Str("guid", item.Guid).
		Msg("Built prompt for Gemini API")

	// Call the Gemini API
	startTime := time.Now()
	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Error().
			Err(err).
			Str("guid", item.Guid).
			Dur("duration", time.Since(startTime)).
			Msg("Error calling Gemini API")
		return nil, fmt.Errorf("error calling Gemini API: %w", err)
	}

	log.Debug().
		Str("guid", item.Guid).
		Dur("duration", time.Since(startTime)).
		Msg("Successfully got response from Gemini API")

	// Parse the response into a NewsItem
	newsItem, err := parseGeminiResponse(response, item)
	if err != nil {
		log.Error().
			Err(err).
			Str("guid", item.Guid).
			Msg("Error parsing Gemini response")
		return nil, fmt.Errorf("error parsing Gemini response: %w", err)
	}

	return newsItem, nil
}

func (g *GeminiClient) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	log := logger.Get()
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)
	
	log.Debug().
		Str("model", g.model).
		Msg("Sending request to Gemini API")

	req := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{
				Text: prompt,
			}},
		}},
	}

	log.Debug().
		Interface("request", req).
		Msg("Sending Gemini API request")

	resp, err := g.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&geminiResponse{}).
		Post(url)

	log.Debug().
		Int("status_code", resp.StatusCode()).
		Str("status", resp.Status()).
		Msg("Received Gemini API response")

	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp.Body(), &errResp); err == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	var result geminiResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", fmt.Errorf("failed to parse API response: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

func buildPrompt(item models.FeedItem) string {
	return fmt.Sprintf(`
You are a professional Reuters news editor rewriting Turkish news into clear, factual, and SEO-optimized English.

---

### 🧱 STRICT RULES (Follow Exactly)
1. Always write "Türkiye", never "Turkey".
2. Preserve all proper nouns (e.g., "İstanbul", "Ankara", "Recep Tayyip Erdoğan"). News articles should be written for readers who do not speak Turkish and speak English. Keep this in mind.
3. The news body ("content_md") must contain only the rewritten article text.
   - Exclude titles, dates, author lines, or metadata.
   - Write in Reuters-style: concise, neutral, and fact-driven.
   - Use Markdown formatting. Bold, italic, and other formatting should be used sparingly.
   - Include "##" subheadings where logically needed.
   - Keep paragraphs short (2–3 sentences).
   - Maintain quotes accurately.
4. Generate up to 3 tags based only on proper nouns. This field is REQUIRED and must not be empty. Include names of people, organizations, and locations mentioned in the article. Do not create tags such as “Turkey,” “Turkiye,” or “Türkiye.”
5. Add SEO fields:
   - "seo_title": under 60 characters. The title is very important. After creating the title, do another grammar check. 
   - "seo_description": 120–160 characters.
6. Never invent facts. Summarize only what is known.

---

### 🧠 RESPONSE FORMAT
Return a valid JSON object only (no markdown fences):

{
  "seo_title": "Concise, factual SEO title under 60 characters",
  "seo_description": "Clear summary between 120–160 characters",
  "content_md": "Rewritten English article body in Markdown, with ## subheadings where needed",
  "tags": ["Ankara", "Baykar", "Recep Tayyip Erdoğan", "Ministry of Health"]
}

---

### 📰 SOURCE ARTICLE (Turkish)
Title: %s
Content: %s

Now produce the JSON output following all rules exactly.
`, escapeJSON(item.TitleTR), escapeJSON(item.ContentTR))
}


func parseGeminiResponse(response string, item models.FeedItem) (*models.NewsItem, error) {
	var result struct {
		SeoTitle    string   `json:"seo_title"`
		SeoDesc     string   `json:"seo_description"`
		ContentMD   string   `json:"content_md"`
		Tags        []string `json:"tags"`
	}

	// Clean the response (sometimes Gemini adds markdown code blocks)
	cleanResponse := strings.TrimSpace(response)
	if strings.HasPrefix(cleanResponse, "```json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```json\n")
		cleanResponse = strings.TrimSuffix(cleanResponse, "\n```")
	}

	if err := json.Unmarshal([]byte(cleanResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nResponse: %s", err, cleanResponse)
	}

	// Ensure tags is never nil
	tags := make([]string, 0)
	if result.Tags != nil {
		tags = result.Tags
	}

	// Validate that we have the required number of tags
	if len(tags) < 3 {  // Require at least 3 tags
		return nil, fmt.Errorf("insufficient tags generated: expected at least 3, got %d", len(tags))
	}

	// Create and return the news item
	return &models.NewsItem{
		ID:          generateID(),
		SourceGuid:  item.Guid,
		SeoTitle:    result.SeoTitle,
		SeoDesc:     result.SeoDesc,
		ContentMD:   result.ContentMD,
		Category:    item.Category, // Use category from smart feed extraction
		Tags:        tags,
		Image:       item.Image,
		OriginalUrl: item.Url,
		CreatedAt:   time.Now(),
	}, nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

func generateID() string {
	// In a real application, you might want to use UUID or another unique ID generator
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
