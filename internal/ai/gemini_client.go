// internal/ai/gemini_client.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

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

// generationConfig, Gemini API'ye gönderilecek parametreleri tanımlar.
type generationConfig struct {
	Temperature     float32 `json:"temperature"`
	TopP            float32 `json:"topP"`
	MaxOutputTokens int32   `json:"maxOutputTokens"`
}

type geminiRequest struct {
	Contents         []geminiContent   `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"` // Eklendi
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
		SetTimeout(60 * time.Second). // Timeout artırıldı (30sn bazen yetmeyebilir)
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

	// Set a timeout context - 30s API için kısa olabilir, 45sn'ye çıkarıldı.
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
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
			Str("raw_response", response). // Hata ayıklama için raw response'u logla
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

	// Best practice ayarları eklendi
	genConfig := &generationConfig{
		Temperature:     0.4,
		TopP:            0.95,
		MaxOutputTokens: 4096, // İçerik + JSON için yeterli alan
	}

	req := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{
				Text: prompt,
			}},
		}},
		GenerationConfig: genConfig, // GenerationConfig eklendi
	}

	log.Debug().
		Interface("request_body_gofmt", fmt.Sprintf("%#v", req)). // Loglamayı iyileştir
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

	// Hata mesajını da kontrol et
	if result.Error != nil && result.Error.Message != "" {
		return "", fmt.Errorf("API returned error: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		// Güvenlik veya başka bir nedenle içerik engellenmiş olabilir.
		log.Warn().
			Str("response_body", resp.String()).
			Msg("No content in Gemini response, possibly blocked or empty.")
		return "", fmt.Errorf("no content in response (possibly blocked by safety settings or empty)")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// *** YENİ PROMPT ***
func buildPrompt(item models.FeedItem) string {
	return fmt.Sprintf(`You are a professional senior news editor. Your task is to translate and rewrite Turkish news for a global audience in a **Reuters-style**: neutral, factual, and objective.

Your goal is to take the source Turkish news and generate a **strictly valid JSON object** based on the following workflow and rules.

---

### STEP 1: WRITE THE ARTICLE (content_md)

First, write the full English news article based on the source.

* **Tone:** Maintain a neutral, objective, and factual Reuters-style journalistic tone.
* **Language:** Use flawless English. **Always** use "Türkiye", never "Turkey".
* **Quality:** This must be a full news article, not a short summary. Aim for 4-5 paragraphs (approx. 150-250 words) to ensure proper coverage of the topic.
* **Markdown Rules:**
    * Use '##' subheadings where logically appropriate to structure the article.
    * Use '*italic text*' for all direct quotes.
    * **DO NOT** use bold formatting ('**...**') anywhere in the article content.
    * **DO NOT** add a '# Title' or any main title at the beginning of the 'content_md' field.

---

### STEP 2: GENERATE METADATA

Based on the English article you wrote in Step 1, generate the following fields:

* **seo_title:** A concise, engaging English SEO title (under 60 characters).
* **seo_description:** A concise English summary for SEO (120-160 characters).
* **category:**
    * Select **ONLY ONE** category from this **exact list**:
    * '["Türkiye", "Business", "World", "Technology", "Sports", "Entertainment"]'
* **tags:**
    * Generate 3-5 relevant tags.
    * Focus on specific entities: people, organizations, and locations (cities, countries).
    * **DO NOT** use "Türkiye" as a tag.
    * Avoid generic tags like "News" or "Update".
* **featured:**
    * Set to 'true' if the news has a significant impact or is of interest to a wide audience.
    * Otherwise, set to 'false'.

---

### STEP 3: FORMAT THE OUTPUT

Combine all elements into a **single, valid JSON object**. Do not include any text, notes, or explanations before or after the JSON structure.

{
  "category": "...",
  "seo_title": "...",
  "seo_description": "...",
  "content_md": "...",
  "tags": ["...", "..."],
  "featured": false
}

---

### SOURCE NEWS (Turkish)
Title: %s
Content: %s

Now, generate the JSON output following all rules.
`, escapeJSON(item.TitleTR), escapeJSON(item.ContentTR))
}

func parseGeminiResponse(response string, item models.FeedItem) (*models.NewsItem, error) {
	var result struct {
		Category   string   `json:"category"`
		SeoTitle   string   `json:"seo_title"`
		SeoDesc    string   `json:"seo_description"`
		ContentMD  string   `json:"content_md"`
		Tags       []string `json:"tags"`
		Featured   bool     `json:"featured"`
	}

	// Clean Gemini's JSON output from markdown code blocks and non-breaking spaces
	cleanResponse := strings.TrimSpace(response)

	// Handle ```json ... ``` format
	if strings.HasPrefix(cleanResponse, "```json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```json")
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	} else if strings.HasPrefix(cleanResponse, "```") {
		// Handle case where it's just ``` ... ```
		cleanResponse = strings.TrimPrefix(cleanResponse, "```")
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
	}

	// Clean any remaining whitespace and newlines
	cleanResponse = strings.TrimSpace(cleanResponse)

	// If the response still starts with "json" (some Gemini versions do this)
	if strings.HasPrefix(cleanResponse, "json") {
		cleanResponse = strings.TrimSpace(strings.TrimPrefix(cleanResponse, "json"))
	}

	// Replace non-breaking spaces with regular spaces
	cleanResponse = strings.ReplaceAll(cleanResponse, "\u00A0", " ")
	cleanResponse = strings.ReplaceAll(cleanResponse, " ", " ") // Non-breaking space in different format

	// Clean up any other potential invisible characters
	cleanResponse = strings.Map(func(r rune) rune {
		if r == '\u00A0' || r == '\u200B' {
			return ' ' // Replace with space
		}
		if unicode.IsSpace(r) && r != ' ' && r != '\n' && r != '\t' {
			return ' ' // Replace other unusual spaces with regular space
		}
		return r
	}, cleanResponse)

	// Remove any double spaces that might have been created
	for strings.Contains(cleanResponse, "  ") {
		cleanResponse = strings.ReplaceAll(cleanResponse, "  ", " ")
	}

	// Handle markdown bold/italic syntax in JSON content (artık prompt'ta bold istemiyoruz ama italic kalabilir)
	// Bu temizlikler JSON parse hatasını engellemek için.
	cleanResponse = strings.ReplaceAll(cleanResponse, "**", "")
	// Bazen JSON değerleri içine *italic* koyabiliyor, bunu da temizleyelim.
	// cleanResponse = strings.ReplaceAll(cleanResponse, "*", "") // Bu content_md'deki italic'leri de bozar. Dikkatli olmalı.
	// Sadece JSON yapısını bozacak yerlerdeki *'ları temizlemek daha zor.
	// Şimdilik prompt'a güvendiğimiz için bu adımı atlayabiliriz.

	// Try to decode JSON
	if err := json.Unmarshal([]byte(cleanResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini JSON: %w\nResponse: %s", err, cleanResponse)
	}

	// Validate minimal required fields
	if result.SeoTitle == "" || result.ContentMD == "" {
		return nil, fmt.Errorf("missing required fields in Gemini output (SeoTitle or ContentMD)")
	}

	// Use AI-detected category if available; otherwise fallback to feed's category
	category := strings.TrimSpace(result.Category)
	if category == "" {
		category = item.Category
	}
	// *** YENİ: Kategori Normalleştirme (istediğiniz gibi) ***
	// Normalize category: enforce lowercase and convert Türkiye variants to 'turkiye'
	if strings.EqualFold(category, "Türkiye") || strings.EqualFold(category, "türkiye") {
		category = "turkiye"
	} else {
		category = strings.ToLower(category)
	}

	// *** YENİ: Etiket (Tag) Temizleme ***
	var finalTags []string
	// Kaldırılacak etiketlerin küçük harf haritası
	tagsToStrip := map[string]bool{
		"türkiye": true,
		"turkiye": true,
		"turkey":  true,
	}

	if result.Tags != nil {
		for _, tag := range result.Tags {
			trimmedTag := strings.TrimSpace(tag)
			if trimmedTag == "" {
				continue // Boş etiketleri atla
			}

			normalizedTag := strings.ToLower(trimmedTag)

			// Eğer etiket "strip" listesinde DEĞİLSE ekle
			if _, exists := tagsToStrip[normalizedTag]; !exists {
				finalTags = append(finalTags, trimmedTag)
			}
		}
	}

	// Hiç etiket kalmadıysa veya hiç gelmediyse bir fallback ata
	if len(finalTags) == 0 {
		finalTags = []string{"General"}
	}

	// Create and return the news item
	return &models.NewsItem{
		ID:          generateID(),
		SourceGuid:  item.Guid,
		SeoTitle:    result.SeoTitle,
		SeoDesc:     result.SeoDesc,
		ContentMD:   result.ContentMD,
		Category:    category,  // Normalize edilmiş kategori
		Tags:        finalTags, // Temizlenmiş etiket listesi
		Image:       item.Image,
		OriginalUrl: item.Url,
		Featured:    &result.Featured,
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