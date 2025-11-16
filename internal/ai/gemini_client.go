// internal/ai/gemini_client.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bilgisen/goen/internal/logger"
	"github.com/bilgisen/goen/internal/models"
	"github.com/go-resty/resty/v2"
)

// ---------- GeminiClient ve tipler ----------
type GeminiClient struct {
	client    *resty.Client
	apiKey    string
	model     string
	baseURL   string
	limiter   *RedisLimiter // optional
	tpmLimit  int           // tokens per minute limit
}

// generationConfig: Gemini API parametreleri
type generationConfig struct {
	Temperature     float32 `json:"temperature"`
	TopP            float32 `json:"topP"`
	TopK            int32   `json:"topK"`
	MaxOutputTokens int32   `json:"maxOutputTokens"`
}

type geminiRequest struct {
	Contents         []geminiContent   `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"`
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

// NewGeminiClient creates a new Gemini client with optional Redis based limiter.
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

// GenerateEnglishNews: public entry point
func (g *GeminiClient) GenerateEnglishNews(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	if g.limiter != nil {
		estimatedTokens := estimateTokens(item.ContentTR)
		if err := g.limiter.WaitIfNeeded(ctx, estimatedTokens); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}
	}

	log := logger.Get()
	log.Info().Str("guid", item.Guid).Str("title", item.TitleTR).Msg("Starting to process news item")

	// API timeout: 45s (en kötü durumda retry'ler var)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
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

		// Do not retry on clear validation errors
		if strings.Contains(lastErr.Error(), "missing required field") ||
			strings.Contains(lastErr.Error(), "content too short") {
			break
		}

		// backoff
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}

	if lastErr != nil {
		log.Error().Err(lastErr).Str("guid", item.Guid).Msg("Failed to generate English news after all retries")
		return nil, lastErr
	}

	log.Info().Str("guid", item.Guid).Str("title", newsItem.SeoTitle).Msg("Successfully processed news item")
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
	log.Debug().Str("guid", item.Guid).Dur("duration", time.Since(startTime)).Msg("Successfully got response from Gemini API")

	newsItem, err := parseGeminiResponse(response, item)
	if err != nil {
		log.Error().
			Err(err).
			Str("guid", item.Guid).
			Str("raw_response_excerpt", truncate(response, 1000)).
			Msg("Error parsing Gemini response")
		return nil, fmt.Errorf("error parsing Gemini response: %w", err)
	}

	return newsItem, nil
}

func (g *GeminiClient) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	log := logger.Get()
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)
	log.Debug().Str("model", g.model).Msg("Sending request to Gemini API")

	// Production-optimized config for low hallucination, stable JSON
	genConfig := &generationConfig{
		Temperature:     0.2,
		TopP:            0.8,
		TopK:            50,
		MaxOutputTokens: 4096,
	}

	req := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{Text: prompt}},
		}},
		GenerationConfig: genConfig,
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
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	var result geminiResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		// Try to recover with fallback: sometimes response JSON shape differs; return raw body
		return string(resp.Body()), nil
	}

	if result.Error != nil && result.Error.Message != "" {
		return "", fmt.Errorf("API returned error: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		log.Warn().Str("response_body", resp.String()).Msg("No content in Gemini response")
		return "", fmt.Errorf("no content in response")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// ---------- PARSE GEMINI RESPONSE (tolerant) ----------
func parseGeminiResponse(response string, item models.FeedItem) (*models.NewsItem, error) {
	// 1) Normalize response
	clean := normalizeResponse(response)

	// 2) Try direct unmarshal
	var result struct {
		SeoTitle      string   `json:"seo_title"`
		SeoDesc       string   `json:"seo_description"`
		ContentMD     string   `json:"content_md"`
		Tags          []string `json:"tags"`
		Peoples       []string `json:"peoples"`
		Locations     []string `json:"locations"`
		Organizations []string `json:"organizations"`
		Featured      bool     `json:"featured"`
	}

	// Attempt direct unmarshal
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		// validate essential fields
		if strings.TrimSpace(result.SeoTitle) == "" || strings.TrimSpace(result.ContentMD) == "" {
			return nil, fmt.Errorf("missing required fields in Gemini output (seo_title or content_md empty)")
		}
		return buildNewsItemFromResult(result, item), nil
	}

	// 3) If direct unmarshal fails, try extracting first {...} JSON object using bracket scanning
	jsonBlock, err := extractJSONBlock(clean)
	if err == nil {
		// try unmarshal extracted block
		if err2 := json.Unmarshal([]byte(jsonBlock), &result); err2 == nil {
			if strings.TrimSpace(result.SeoTitle) == "" || strings.TrimSpace(result.ContentMD) == "" {
				return nil, fmt.Errorf("missing required fields after extracting JSON block")
			}
			return buildNewsItemFromResult(result, item), nil
		}
		// attempt to "fix" common JSON problems and retry
		fixed := tryFixCommonJSONIssues(jsonBlock)
		if fixed != "" {
			if err3 := json.Unmarshal([]byte(fixed), &result); err3 == nil {
				if strings.TrimSpace(result.SeoTitle) == "" || strings.TrimSpace(result.ContentMD) == "" {
					return nil, fmt.Errorf("missing required fields after fixing JSON")
				}
				return buildNewsItemFromResult(result, item), nil
			}
		}
	}

	// 4) Last-resort: attempt to find key-value pairs heuristically (best-effort)
	heuristic, herr := heuristicParse(clean)
	if herr == nil && heuristic != nil {
		// require content_md and seo_title present
		if strings.TrimSpace(heuristic.ContentMD) != "" && strings.TrimSpace(heuristic.SeoTitle) != "" {
			return buildNewsItemFromResult(*heuristic, item), nil
		}
	}

	// 5) If everything fails, return original response for debugging
	return nil, fmt.Errorf("failed to parse Gemini JSON output after multiple attempts; raw: %.800s", clean)
}

// normalizeResponse: remove markdown fences, odd prefixes, NBSPs, control chars
func normalizeResponse(s string) string {
	out := strings.TrimSpace(s)
	// remove common markdown code fences
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	out = strings.TrimSpace(out)
	// sometimes model outputs "json\n{...}"
	if strings.HasPrefix(strings.ToLower(out), "json") {
		out = strings.TrimSpace(out[4:])
	}
	// replace non-breaking spaces
	out = strings.ReplaceAll(out, "\u00A0", " ")
	out = strings.ReplaceAll(out, "\u200B", "")
	// remove weird control chars except newline and tab
	out = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, out)
	// collapse repeated spaces
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(out)
}

// extractJSONBlock finds the first balanced JSON object in the string using stack-based scan
func extractJSONBlock(s string) (string, error) {
	start := -1
	depth := 0
	inString := false
	escape := false

	for i, ch := range s {
		if start == -1 && ch == '{' {
			start = i
			depth = 1
			continue
		}
		if start >= 0 {
			if ch == '"' && !escape {
				inString = !inString
			}
			if ch == '\\' && !escape {
				escape = true
			} else {
				escape = false
			}
			if !inString {
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
					if depth == 0 {
						return s[start : i+1], nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no balanced json block found")
}

// tryFixCommonJSONIssues: remove trailing commas, replace single quotes around keys/strings (best-effort)
func tryFixCommonJSONIssues(s string) string {
	fixed := s
	// remove trailing commas before } or ]
	reTrailingComma := regexp.MustCompile(`,(\s*[}\]])`)
	fixed = reTrailingComma.ReplaceAllString(fixed, "$1")
	// Sometimes model uses single quotes for strings - convert only if appears to be JSON-like (risky but helpful)
	// Convert single-quoted values to double quotes when safe: '...'
	reSingleQuotes := regexp.MustCompile(`'([^']*)'`)
	fixed = reSingleQuotes.ReplaceAllStringFunc(fixed, func(m string) string {
		// if m contains a double quote, skip to avoid breaking JSON
		if strings.Contains(m, "\"") {
			return m
		}
		// replace surrounding single quotes with double quotes, escape inner double quotes
		inner := m[1 : len(m)-1]
		inner = strings.ReplaceAll(inner, "\"", "\\\"")
		return `"` + inner + `"`
	})
	// Trim unescaped newlines in JSON values (replace with \n)
	// This is conservative: wrap only if looks like "key": "multi
	// line..."
	reUnescapedNewline := regexp.MustCompile(`"([^"]*)"\s*:\s*"([^"]*\n[^"]*)"`)
	fixed = reUnescapedNewline.ReplaceAllStringFunc(fixed, func(m string) string {
		// a simpler but safe approach: replace literal newlines with \n
		return strings.ReplaceAll(m, "\n", `\n`)
	})
	return fixed
}

// heuristicParse: very best-effort extraction if JSON completely failed
func heuristicParse(s string) (*struct {
	SeoTitle      string   `json:"seo_title"`
	SeoDesc       string   `json:"seo_description"`
	ContentMD     string   `json:"content_md"`
	Tags          []string `json:"tags"`
	Peoples       []string `json:"peoples"`
	Locations     []string `json:"locations"`
	Organizations []string `json:"organizations"`
	Featured      bool     `json:"featured"`
}, error) {

	// simple regexes for seo_title and content_md as last resort
	reSeo := regexp.MustCompile(`"seo_title"\s*:\s*"([^"]+)"`)
	reContent := regexp.MustCompile(`"content_md"\s*:\s*"([^"]+)"`)

	foundSeo := reSeo.FindStringSubmatch(s)
	foundContent := reContent.FindStringSubmatch(s)

	var out struct {
		SeoTitle      string   `json:"seo_title"`
		SeoDesc       string   `json:"seo_description"`
		ContentMD     string   `json:"content_md"`
		Tags          []string `json:"tags"`
		Peoples       []string `json:"peoples"`
		Locations     []string `json:"locations"`
		Organizations []string `json:"organizations"`
		Featured      bool     `json:"featured"`
	}

	if len(foundSeo) > 1 {
		out.SeoTitle = foundSeo[1]
	}
	if len(foundContent) > 1 {
		out.ContentMD = foundContent[1]
	}

	// If neither found, fail
	if out.SeoTitle == "" && out.ContentMD == "" {
		return nil, fmt.Errorf("heuristic parse failed")
	}
	return &out, nil
}

// buildNewsItemFromResult: convert parsed struct to models.NewsItem
func buildNewsItemFromResult(res interface{}, item models.FeedItem) *models.NewsItem {
	// Convert via marshal/unmarshal for ease (res type is known earlier)
	b, _ := json.Marshal(res)
	var r struct {
		SeoTitle      string   `json:"seo_title"`
		SeoDesc       string   `json:"seo_description"`
		ContentMD     string   `json:"content_md"`
		Tags          []string `json:"tags"`
		Peoples       []string `json:"peoples"`
		Locations     []string `json:"locations"`
		Organizations []string `json:"organizations"`
		Featured      bool     `json:"featured"`
	}
	_ = json.Unmarshal(b, &r)

	// Clean tags: strip trivial country tags
	tagsToStrip := map[string]bool{
		"türkiye": true, "turkiye": true, "turkey": true,
	}
	var finalTags []string
	for _, t := range r.Tags {
		tt := strings.TrimSpace(t)
		if tt == "" {
			continue
		}
		if _, ok := tagsToStrip[strings.ToLower(tt)]; ok {
			continue
		}
		finalTags = append(finalTags, tt)
	}
	if len(finalTags) == 0 {
		finalTags = []string{"General"}
	}

	// Category normalization from feed item
	category := strings.TrimSpace(item.Category)
	if strings.EqualFold(category, "Türkiye") || strings.EqualFold(category, "türkiye") {
		category = "turkiye"
	} else {
		category = strings.ToLower(category)
	}

	return &models.NewsItem{
		ID:            generateID(),
		SourceGuid:    item.Guid,
		SeoTitle:      r.SeoTitle,
		SeoDesc:       r.SeoDesc,
		ContentMD:     r.ContentMD,
		Category:      category,
		Tags:          finalTags,
		Peoples:       r.Peoples,
		Locations:     r.Locations,
		Organizations: r.Organizations,
		Image:         item.Image,
		OriginalUrl:   item.Url,
		Featured:      &r.Featured,
		CreatedAt:     time.Now(),
	}
}

// ---------- UTILITIES ----------

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

// ---------- PERFECT PROMPT (AŞAMA 2) ----------
func buildPrompt(item models.FeedItem) string {
	// Use escaped content when embedding into prompt
	escaped := escapeJSON(item.ContentTR)
	return fmt.Sprintf(`
You are a professional senior news editor. Your task is to translate and rewrite Turkish news for a global audience in a Reuters-style: neutral, factual, and objective.

Your job: Take the Turkish source text below and generate a *strictly valid JSON object* following the steps and structure.

---

### STEP 1 — content_md

Write a full English news article based on the Turkish source.

- Use a neutral, factual Reuters reporting tone.
- Use flawless English.
- Always use "Türkiye", never "Turkey".
- Write 4–5 paragraphs (~150–250 words).
- Markdown rules:
  - Use "##" subheadings where appropriate.
  - Use *italic* formatting for all quotes.
  - Never use bold formatting.
  - Do NOT add a main title.

---

### STEP 2 — METADATA

Generate:

- seo_title: under 60 chars
- seo_description: 120–160 chars
- tags: 3–5 topic tags (Title Case)
- peoples: list notable people or []
- locations: list countries/cities or []
- organizations: list orgs or []
- featured: true/false

Rules:
* DO NOT add any information not explicitly present in the Turkish source.
* Do NOT infer roles or backgrounds (e.g., "former president") unless the source explicitly states them.
* NEVER set featured to true by default. Only set to true when the article describes:
  - a major political event with nationwide or international consequences
  - a large-scale disaster or accident
  - a significant economic development affecting markets or many people
  - or other large-impact items clearly described in the source

---

### STEP 3 — FINAL JSON OUTPUT FORMAT

Return ONLY this JSON (no text outside):

{
  "content_md": "...",
  "seo_title": "...",
  "seo_description": "...",
  "tags": [...],
  "peoples": [...],
  "locations": [...],
  "organizations": [...],
  "featured": false
}

---

### SOURCE TEXT (Turkish):
%s
`, escaped)
}