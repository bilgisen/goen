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
	limiter   *RedisLimiter
	tpmLimit  int
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

func NewGeminiClient(apiKey, model string, rpm, tpm int, redisURL string) (*GeminiClient, error) {
	var limiter *RedisLimiter
	var err error

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
		AddRetryCondition(func(r *resty.Response, err error) bool {
			return r.StatusCode() == 429 || (r.StatusCode() >= 500 && r.StatusCode() < 600)
		})

	return &GeminiClient{
		client:   client,
		apiKey:   apiKey,
		model:    model,
		baseURL:  "https://generativelanguage.googleapis.com/v1beta/models",
		limiter:  limiter,
		tpmLimit: tpm,
	}, nil
}

func (g *GeminiClient) GenerateEnglishNews(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	if g.limiter != nil {
		estimatedTokens := estimateTokens(item.ContentTR)
		if err := g.limiter.WaitIfNeeded(ctx, estimatedTokens); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}
	}

	log := logger.Get()
	log.Info().Str("guid", item.Guid).Str("title", item.TitleTR).Msg("Starting to process news item")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var newsItem *models.NewsItem
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			log.Debug().Str("guid", item.Guid).Int("attempt", attempt).Msg("Retrying news processing")
		}

		newsItem, lastErr = g.generateEnglishNewsOnce(ctx, item)
		if lastErr == nil {
			break
		}

		// Don't retry on validation errors
		if strings.Contains(lastErr.Error(), "missing required field") ||
			strings.Contains(lastErr.Error(), "content too short") {
			break
		}

		if attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}

	if lastErr != nil {
		log.Error().Err(lastErr).Str("guid", item.Guid).Msg("Failed to generate English news")
		return nil, lastErr
	}

	log.Info().
		Str("guid", item.Guid).
		Str("title", newsItem.SeoTitle).
		Str("category", newsItem.Category).
		Strs("tags", newsItem.Tags).
		Msg("Successfully processed news item")

	return newsItem, nil
}

func (g *GeminiClient) generateEnglishNewsOnce(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	prompt := buildPrompt(item)
	log := logger.Get()
	log.Debug().Str("guid", item.Guid).Msg("Built prompt for Gemini API")

	startTime := time.Now()
	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		log.Error().Err(err).Str("guid", item.Guid).Dur("duration", time.Since(startTime)).Msg("Error calling Gemini API")
		return nil, fmt.Errorf("error calling Gemini API: %w", err)
	}

	log.Debug().Str("guid", item.Guid).Dur("duration", time.Since(startTime)).Msg("Got response from Gemini")

	newsItem, err := parseGeminiResponse(response, item)
	if err != nil {
		log.Error().Err(err).Str("guid", item.Guid).Msg("Error parsing Gemini response")
		return nil, fmt.Errorf("error parsing Gemini response: %w", err)
	}

	// Log parsed info for debug
	log.Debug().
		Str("guid", item.Guid).
		Str("category", newsItem.Category).
		Strs("tags", newsItem.Tags).
		Msg("Parsed Gemini output")

	return newsItem, nil
}

func (g *GeminiClient) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)

	req := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
	}

	resp, err := g.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&geminiResponse{}).
		Post(url)

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

// buildPrompt returns the optimized prompt (final version) to send to the LLM.
// It includes the exact category list and tag-generation rules you approved.
func buildPrompt(item models.FeedItem) string {
	return fmt.Sprintf(`
You are a *professional Reuters-style English news editor. Your task is to **translate and rewrite Turkish news entirely in English*, following strict editorial, style, and formatting rules.

---

### LANGUAGE RULES
- Write entirely in *English*.
- Never use Turkish words or sentences in any field.
- Translate all Turkish titles and terms if an English equivalent exists (e.g., "Cumhurbaşkanı" → "President").
- Preserve proper Turkish nouns as-is (e.g., "İstanbul", "Ankara", "Recep Tayyip Erdoğan").
- *Always use “Türkiye” instead of “Turkey” in all parts of the article, including titles, content, and subheadings.*
- Use correct grammar and Reuters-style journalistic tone — neutral, factual, and concise.
- Each paragraph should have *2–3 sentences*.

---

### STYLE RULES
- Use *Markdown* for the full article body.
- Highlight all *nouns* in bold.
- Write all *quotes* in italic.
- Include "##" subheadings where relevant.
- Do not invent or add information.
- Keep the writing objective and report-like.

---

### CATEGORY SELECTION RULES
Select *only one* category from the following list:
["Türkiye", "Business", "World", "Technology", "Sports", "Entertainment"]

- Choose "Türkiye" only if the news primarily covers domestic issues, politics, or local events.
- Choose "Business" for finance, economy, company, or trade-related topics.
- Choose "World" for international developments not specific to Türkiye.
- Choose "Technology" for digital innovation, science, or tech company news.
- Choose "Sports" for athletic or competition-related stories.
- Choose "Entertainment" for arts, culture, or lifestyle-related content.
- Return the category as a lowercase English word (e.g., "business", "sports").

---

### TAG GENERATION RULES
Suggest *3–6 relevant tags* focusing on main topics, entities, and places mentioned in the news.

- Include country names if they are relevant to the article.
- Avoid personal initials, honorifics (Mr., Dr., etc.), or media outlets.
- Use concise one- or two-word tags (e.g., "Economy", "Recep Tayyip Erdoğan", "Artificial Intelligence").
- Do not repeat generic tags such as "News" or "Update".

---

### OUTPUT FORMAT (JSON only)
Return *strictly valid JSON* with the following structure:

{
  "category": "business",
  "seo_title": "English SEO title under 60 characters",
  "seo_description": "English description (120–160 chars)",
  "content_md": "Full rewritten English article in Markdown (with *bold* nouns and italic quotes)",
  "tags": ["Economy", "Recep Tayyip Erdoğan", "Trade Relations"]
}

---

### SOURCE ARTICLE (Turkish)
Title: %s
Content: %s

Now, produce the JSON output following all rules exactly.
`, escapeJSON(item.TitleTR), escapeJSON(item.ContentTR))
}

func parseGeminiResponse(response string, item models.FeedItem) (*models.NewsItem, error) {
	var result struct {
		SeoTitle  string   `json:"seo_title"`
		SeoDesc   string   `json:"seo_description"`
		ContentMD string   `json:"content_md"`
		Category  string   `json:"category"`
		Tags      []string `json:"tags"`
	}

	cleanResponse := strings.TrimSpace(response)
	if strings.HasPrefix(cleanResponse, "json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "json")
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
		cleanResponse = strings.TrimSpace(cleanResponse)
	}

	if err := json.Unmarshal([]byte(cleanResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nResponse: %s", err, cleanResponse)
	}

	// Category: normalize to lowercase; fallback to "world"
	category := strings.ToLower(strings.TrimSpace(result.Category))
	if category == "" {
		category = "world"
	}

	// Tags: sanitize, but be lenient (don't fail if few tags).
	// Remove empty tags and skip initials like "Z.D."
	validTags := make([]string, 0)
	for _, tag := range result.Tags {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		// Skip initials like Z.D., A.G.
		if len(t) <= 5 && strings.Count(t, ".") >= 1 {
			continue
		}
		// Normalize to Title Case (simple approach)
		validTags = append(validTags, toTitleCase(t))
	}

	// Build NewsItem (no longer erroring if tags < 3)
	return &models.NewsItem{
		ID:          generateID(),
		SourceGuid:  item.Guid,
		SeoTitle:    result.SeoTitle,
		SeoDesc:     result.SeoDesc,
		ContentMD:   result.ContentMD,
		Category:    category,
		Tags:        validTags,
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
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// toTitleCase makes a simple Title Case for each word in the tag.
// This is a lightweight helper; it's OK for short tag strings.
func toTitleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// split on spaces and capitalize first rune of each word
	parts := strings.Fields(s)
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		first := string(runes[0])
		rest := ""
		if len(runes) > 1 {
			rest = string(runes[1:])
		}
		parts[i] = strings.ToUpper(strings.ToLower(first)) + strings.ToLower(rest)
	}
	return strings.Join(parts, " ")
}