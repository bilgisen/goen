🧱 PROJECT NAME

ai-news-processor

🧩 SYSTEM OVERVIEW

Amaç:
Türkçe JSON feed URL’lerini alır → her item için unique hash üretir → Redis cache’de duplicate kontrolü yapar → AI (Gemini) ile İngilizce haber üretir → JSON olarak kaydeder → Next.js uygulamasına İngilizce feed endpoint’leri sunar.

⚙️ TECH STACK
Layer	Tech
Language	Go (Golang 1.23+)
Framework	Fiber (HTTP)
Database	Redis (cache + processed hashes)
Storage	Local JSON files (/data/processed/) veya Cloudflare R2 / Supabase storage
AI	Google Gemini API
Hash	SHA256 (built-in crypto/sha256)
JSON Handling	encoding/json
HTTP Client	resty
Scheduling	robfig/cron/v3
Logging	zerolog
Config	.env + godotenv
📁 FOLDER STRUCTURE
ai-news-processor/
├── cmd/
│   └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── feed/
│   │   ├── fetcher.go
│   │   ├── parser.go
│   │   └── processor.go
│   ├── ai/
│   │   ├── gemini_client.go
│   │   ├── prompt_templates.go
│   │   └── postprocessor.go
│   ├── cache/
│   │   └── redis.go
│   ├── storage/
│   │   ├── writer.go
│   │   ├── reader.go
│   │   └── file_utils.go
│   ├── utils/
│   │   ├── hasher.go
│   │   └── logger.go
│   ├── models/
│   │   ├── feed_item.go
│   │   └── news_item.go
│   └── api/
│       ├── server.go
│       └── routes.go
├── data/
│   ├── feeds/        # TR feed source JSONs
│   └── processed/    # EN processed JSONs
├── scripts/
│   └── run_cron.sh
├── go.mod
├── go.sum
└── .env

📘 FILE-BY-FILE EXPLANATION
/cmd/main.go

Purpose: Entry point.
Task: Initialize config, Redis, scheduler, API routes, and start the cron job.

func main() {
  cfg := config.Load()
  redis := cache.InitRedis(cfg)
  app := fiber.New()
  api.SetupRoutes(app)
  scheduler.Start(redis, cfg)
  app.Listen(cfg.Port)
}

/internal/config/config.go

Purpose: Load .env variables and app configuration.
Keys:

PORT=8080
REDIS_URL=redis://localhost:6379
AI_API_KEY=...
AI_MODEL=gemini-pro
FEED_SOURCE_PATH=./data/feeds/
PROCESSED_PATH=./data/processed/

/internal/models/feed_item.go

Purpose: Represents the Turkish source feed structure.

type FeedItem struct {
  Guid        string `json:"guid"`
  TitleTR     string `json:"title"`
  ContentTR   string `json:"content"`
  Image       string `json:"image"`
  Url         string `json:"url"`
  Category    string `json:"category"`
}

/internal/models/news_item.go

Purpose: Represents the generated English content.

type NewsItem struct {
  Id           string   `json:"id"`
  SourceGuid   string   `json:"source_guid"`
  SeoTitle     string   `json:"seo_title"`
  SeoDesc      string   `json:"seo_description"`
  TLDR         []string `json:"tldr"`
  ContentMD    string   `json:"content_md"`
  Category     string   `json:"category"`
  Tags         []string `json:"tags"`
  ImageTitle   string   `json:"image_title"`
  ImageDesc    string   `json:"image_desc"`
  OriginalUrl  string   `json:"original_url"`
  CreatedAt    string   `json:"created_at"`
}

/internal/feed/fetcher.go

Purpose: Fetch Turkish JSON feed from URLs (Render.com endpoints).
AI Task: none
Logic:

Fetch JSON from Feed URL.

Decode and return []FeedItem.

/internal/feed/parser.go

Purpose: Filter and normalize fetched feeds (clean HTML, etc).
Logic:

Remove HTML tags.

Normalize whitespace.

Return sanitized FeedItem.

/internal/feed/processor.go

Purpose: Main orchestrator.
Logic:

Generate hash := utils.Hash(feedItem.Url)

Check Redis processed:<hash>

If not found → send to ai.GenerateNews(feedItem)

Save output with storage.Writer()

Cache hash in Redis with TTL (30 days)

/internal/ai/gemini_client.go

Purpose: Call Gemini API with structured prompt.
Prompt format:

You are an expert English journalist and SEO writer. 
Transform this Turkish news article into a professional English version with:
- SEO title and description
- TL;DR list (3 bullet points)
- Markdown formatted content
- Relevant tags and category
- Image title and description


Input: FeedItem
Output: NewsItem

/internal/ai/prompt_templates.go

Purpose: Define structured prompts.
E.g.

func BuildPrompt(item models.FeedItem) string {
  return fmt.Sprintf(`Write an English news article based on:
  Title: %s
  Content: %s
  Category: %s`, item.TitleTR, item.ContentTR, item.Category)
}

/internal/ai/postprocessor.go

Purpose: Clean AI output (e.g., convert Markdown, validate TLDR count, etc).

/internal/cache/redis.go

Purpose: Manage connection and duplicate control.
Functions:

IsProcessed(hash string) bool

MarkProcessed(hash string)

/internal/storage/writer.go

Purpose: Save processed NewsItem as JSON file in /data/processed/.
File name: YYYY-MM-DD_<id>.json

/internal/api/server.go & /routes.go

Purpose: Serve the processed English feed for Next.js.
Example endpoint:
GET /api/news/latest → returns combined processed feed JSONs.

/scripts/run_cron.sh

Purpose: Trigger feed fetching & processing periodically (e.g. every 6 hours).

🧠 AI PROCESSING PIPELINE

Fetch feed JSON → /feed/fetcher.go

Parse & sanitize → /feed/parser.go

Hash & deduplicate → /cache/redis.go

AI process (Gemini) → /ai/gemini_client.go

Post-process result → /ai/postprocessor.go

Store as English JSON → /storage/writer.go

Expose via API → /api/server.go

🧩 MODULE LIST SUMMARY
Module	Description
Fiber	Lightweight HTTP API
Resty	Fetch TR JSON feeds
Zerolog	Structured logs
Redis	Cache & deduplication
Cron	Scheduling feeds
Godotenv	Config management
Gemini API	AI text generation
Crypto/SHA256	Hash URLs
OS/Filepath	JSON storage

_________

Aşağıda — senin sistemine uygun olacak şekilde — en kritik iki dosya için production-level Go kod örnekleri var:

1️⃣ feed/processor.go → tüm iş akışını yönetir
2️⃣ ai/gemini_client.go → Gemini API ile etkileşir ve profesyonel İngilizce haber üretir

Her dosyada:

temiz error handling,

concurrent processing,

Redis deduplication kontrolü,

JSON save işlemleri,

AI prompt oluşturma süreci bulunur.

🧩 1. /internal/feed/processor.go
package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ai-news-processor/internal/ai"
	"ai-news-processor/internal/cache"
	"ai-news-processor/internal/models"
	"ai-news-processor/internal/storage"
	"ai-news-processor/internal/utils"
)

func ProcessFeeds(ctx context.Context, feedItems []models.FeedItem, redis *cache.RedisClient) error {
	sem := make(chan struct{}, 5) // Limit concurrency (5 parallel AI calls)
	for _, item := range feedItems {
		item := item
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			hash := utils.Hash(item.Url)

			// Check Redis: Has this feed item been processed before?
			processed, err := redis.IsProcessed(ctx, hash)
			if err != nil {
				log.Printf("Redis error: %v", err)
				return
			}
			if processed {
				log.Printf("Skipping duplicate: %s", item.Url)
				return
			}

			// Build prompt and call Gemini API
			newsItem, err := ai.GenerateEnglishNews(ctx, item)
			if err != nil {
				log.Printf("AI processing failed for %s: %v", item.Url, err)
				return
			}

			newsItem.SourceGuid = item.Guid
			newsItem.OriginalUrl = item.Url
			newsItem.CreatedAt = time.Now().Format(time.RFC3339)

			// Save to file (as JSON)
			fileName := fmt.Sprintf("%s.json", newsItem.Id)
			err = storage.SaveNewsJSON(newsItem, fileName)
			if err != nil {
				log.Printf("Error saving news JSON: %v", err)
				return
			}

			// Mark as processed
			err = redis.MarkProcessed(ctx, hash)
			if err != nil {
				log.Printf("Redis write error: %v", err)
			}

			log.Printf("Processed successfully: %s", newsItem.SeoTitle)
		}()
	}

	// Wait for all goroutines
	time.Sleep(10 * time.Second)
	return nil
}

// Utility for writing combined feed (optional)
func SaveAllToFeedFile(items []models.NewsItem, filePath string) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(filePath, data)
}

🧠 Açıklama:

Her feed item için URL hash alınır (utils.Hash()).

Redis kontrolü yapılır.

Eğer yeni ise Gemini ile İngilizce içerik üretilir.

AI sonucu JSON olarak /data/processed dizinine kaydedilir.

Redis’e işaretlenir (duplicate önleme).

Paralel işleme (5 eş zamanlı AI çağrısı) yapılır.

🤖 2. /internal/ai/gemini_client.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"ai-news-processor/internal/models"
	"ai-news-processor/internal/utils"

	"github.com/go-resty/resty/v2"
)

var (
	apiKey  = os.Getenv("AI_API_KEY")
	apiURL  = "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent"
	client  = resty.New().SetTimeout(30 * time.Second)
)

// Struct for Gemini API request
type geminiRequest struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

// Struct for Gemini API response
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// GenerateEnglishNews calls Gemini API and returns a structured NewsItem
func GenerateEnglishNews(ctx context.Context, item models.FeedItem) (models.NewsItem, error) {
	prompt := buildPrompt(item)

	reqBody := geminiRequest{
		Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{Parts: []struct {
				Text string `json:"text"`
			}{{Text: prompt}}},
		},
	}

	var respBody geminiResponse
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("key", apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetResult(&respBody).
		Post(apiURL)
	if err != nil {
		return models.NewsItem{}, fmt.Errorf("Gemini API error: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return models.NewsItem{}, fmt.Errorf("Gemini response: %v", resp.Status())
	}

	if len(respBody.Candidates) == 0 {
		return models.NewsItem{}, fmt.Errorf("no content returned from Gemini")
	}

	// Parse AI text (Gemini returns text with JSON or Markdown inside)
	rawText := respBody.Candidates[0].Content.Parts[0].Text

	// Try decoding structured JSON first
	var news models.NewsItem
	err = json.Unmarshal([]byte(rawText), &news)
	if err != nil {
		// fallback: manually fill if Gemini returned plain text
		news = models.NewsItem{
			Id:        utils.GenerateUUID(),
			SeoTitle:  fmt.Sprintf("English: %s", item.TitleTR),
			ContentMD: rawText,
			Category:  item.Category,
			Tags:      []string{"ai", "auto-generated"},
		}
	}

	if news.Id == "" {
		news.Id = utils.GenerateUUID()
	}

	return news, nil
}

func buildPrompt(item models.FeedItem) string {
	return fmt.Sprintf(`
You are a professional journalist and SEO expert.
Rewrite the following Turkish news article into a high-quality English version.

Requirements:
- Write a compelling SEO title (max 60 chars)
- Write a meta description (max 150 chars)
- Add 3 bullet TLDR points
- Write a well-structured Markdown content with subheadings
- Generate relevant tags and one category
- Suggest image title and description
- Return valid JSON with fields:
  seo_title, seo_description, tldr, content_md, category, tags, image_title, image_desc

Input:
Title: %s
Content: %s
Category: %s
`, item.TitleTR, item.ContentTR, item.Category)
}

🔧 Destek Dosyaları (özet)

/internal/utils/hasher.go

package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}


/internal/utils/id.go

package utils

import "github.com/google/uuid"

func GenerateUUID() string {
	return uuid.NewString()
}


/internal/storage/writer.go

package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ai-news-processor/internal/models"
)

func SaveNewsJSON(item models.NewsItem, filename string) error {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join("data/processed", filename)
	return os.WriteFile(path, data, 0644)
}

⚙️ Typical Flow

Cron job çalışır (every 6h)

/data/feeds/*.json dosyaları okunur

Her feed item için:

URL hash → Redis check

Gemini çağrısı → İngilizce JSON üretimi

/data/processed/ dizinine kaydet

Redis’e hash yaz → duplicate önlenir

Next.js api/news route bu JSON’ları fetch eder

_____

Mükemmel — o zaman A) cron task scheduler setup’unu yapalım.
Bu aşama, tüm sistemin “kalp atışı” olacak.
Her gün (veya her 6 saatte bir) Go servisin feedleri çekip işlemesini otomatik hale getireceğiz.

Aşağıda sana üretim için hazır (production-ready) bir cron yapısı sunuyorum 👇

🧩 DOSYA: /internal/scheduler/scheduler.go
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"ai-news-processor/internal/cache"
	"ai-news-processor/internal/feed"
	"ai-news-processor/internal/models"
	"ai-news-processor/internal/storage"
	"ai-news-processor/internal/utils"
)

// StartScheduler — main cron job loop
func StartScheduler(redis *cache.RedisClient) {
	c := cron.New(cron.WithSeconds())

	// Run every 6 hours (at minute 00)
	_, err := c.AddFunc("0 0 */6 * * *", func() {
		log.Println("[CRON] Starting feed processing...")
		err := processAllFeeds(redis)
		if err != nil {
			log.Printf("[CRON ERROR] %v\n", err)
		} else {
			log.Println("[CRON] Feed processing completed.")
		}
	})
	if err != nil {
		log.Fatalf("Failed to create cron job: %v", err)
	}

	// Run immediately on startup (first boot)
	go func() {
		log.Println("[BOOT] Initial feed processing...")
		err := processAllFeeds(redis)
		if err != nil {
			log.Printf("[BOOT ERROR] %v\n", err)
		} else {
			log.Println("[BOOT] Initial processing completed.")
		}
	}()

	c.Start()
	log.Println("[CRON] Scheduler started.")
	select {} // block forever
}

func processAllFeeds(redis *cache.RedisClient) error {
	ctx := context.Background()

	// Read all feed sources from folder
	feedFiles, err := storage.ReadFeedFiles("data/feeds")
	if err != nil {
		return err
	}

	for _, file := range feedFiles {
		items, err := storage.ParseFeedFile(file)
		if err != nil {
			log.Printf("Failed to parse %s: %v\n", file, err)
			continue
		}

		log.Printf("[CRON] Processing %d items from %s\n", len(items), file)
		err = feed.ProcessFeeds(ctx, items, redis)
		if err != nil {
			log.Printf("[CRON] Error processing %s: %v\n", file, err)
		}
	}

	return nil
}

🧠 Açıklama

robfig/cron/v3 kullanıyoruz (Go’da en güvenilir scheduler kütüphanesi).

Cron expression: "0 0 */6 * * *" → her 6 saatte bir (00.00, 06.00, 12.00, 18.00).

Servis açılır açılmaz da (boot-time job) ilk çalıştırma yapıyor.

processAllFeeds() fonksiyonu feed dizinindeki tüm JSON dosyaları okuyup feed.ProcessFeeds() fonksiyonunu çağırıyor.

🧰 Destek Dosyaları
/internal/storage/reader.go
package storage

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"ai-news-processor/internal/models"
)

// ReadFeedFiles returns all JSON feed file paths
func ReadFeedFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	return files, nil
}

// ParseFeedFile reads and parses one feed file into FeedItem slice
func ParseFeedFile(path string) ([]models.FeedItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []models.FeedItem
	err = json.Unmarshal(data, &items)
	if err != nil {
		return nil, err
	}

	return items, nil
}

🏁 /cmd/main.go’da entegrasyon
package main

import (
	"ai-news-processor/internal/cache"
	"ai-news-processor/internal/config"
	"ai-news-processor/internal/scheduler"
	"log"
)

func main() {
	cfg := config.Load()
	redis := cache.InitRedis(cfg)

	log.Println("🚀 Starting AI News Processor...")
	scheduler.StartScheduler(redis)
}

🧩 .env Örneği
PORT=8080
REDIS_URL=redis://localhost:6379
AI_API_KEY=YOUR_GEMINI_KEY
FEED_SOURCE_PATH=./data/feeds/
PROCESSED_PATH=./data/processed/
CRON_INTERVAL=*/6

📦 Deploy Strategy (Render / Fly.io / Cloudflare)
Render Build Command:
go build -o server ./cmd/main.go

Start Command:
./server


Render logs üzerinden her 6 saatte bir çalıştığını görebilirsin:

[CRON] Starting feed processing...
[CRON] Processing 10 items from feed_tr.json
...

⚙️ Redis TTL (opsiyonel)

Duplicate kontrolü için Redis key’lerine TTL de ekleyebiliriz:

redis.Set(ctx, "processed:"+hash, true, 30*24*time.Hour)


Bu sayede 30 gün sonra otomatik temizlenir (eski haberlerin tekrar işlenmesine gerek kalmaz).

------

Harika. Şimdi Phase B: Feed Processing & AI Content Generation aşamasına geçiyoruz.
Bu aşamada Go servisimiz Türkçe feed item’larını alacak, Gemini API ile İngilizce SEO-odaklı içerik oluşturacak, ve çıktıyı JSON olarak kaydedecek.

Aşağıda folder yapısı, örnek dosyalar ve AI görev akışı ile birlikte örnek kodlar var 👇

🧩 Folder Structure (Phase B)
/internal
  /ai
    gemini_client.go         # Gemini API wrapper
    content_generator.go     # AI content generation logic
  /processor
    feed_processor.go        # Takes TR JSON, produces EN JSON
  /models
    feed.go                  # Struct definitions for TR and EN feeds
  /utils
    hash.go                  # URL -> hash generator
  /storage
    file_writer.go           # Writes EN JSON files daily
/cmd
  /process
    main.go                  # CLI entrypoint: process feeds
/config
  config.go                  # Config loader (API key, paths)

🧠 AI Task Workflow

feed_processor.go Türkçe feed JSON’larını okur.

Her item için URL hash kontrolü yapılır (işlenmiş mi?).

Gemini API’ye prompt gönderilir:

Amaç: İngilizce profesyonel içerik üretmek.

Prompt: TR metin, başlık, kategori, görsel bilgileri içerir.

AI dönüşünde şu alanlar alınır:

{
  "seo_title": "...",
  "seo_description": "...",
  "tldr": ["...", "...", "..."],
  "markdown_content": "...",
  "category": "...",
  "tags": ["...", "..."],
  "image_title": "...",
  "image_desc": "..."
}


EN JSON daily/2025-10-22.json gibi bir dosyaya kaydedilir.

🧱 Code Examples
/internal/models/feed.go
package models

type FeedItemTR struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Image     string `json:"image"`
	Category  string `json:"category"`
	Timestamp string `json:"timestamp"`
	GUID      string `json:"guid"`
}

type FeedItemEN struct {
	ID              string   `json:"id"`
	GUID            string   `json:"guid"`
	URL             string   `json:"url"`
	SEO_Title       string   `json:"seo_title"`
	SEO_Description string   `json:"seo_description"`
	TLDR            []string `json:"tldr"`
	MarkdownContent string   `json:"markdown_content"`
	Category        string   `json:"category"`
	Tags            []string `json:"tags"`
	ImageTitle      string   `json:"image_title"`
	ImageDesc       string   `json:"image_desc"`
	Timestamp       string   `json:"timestamp"`
}

/internal/utils/hash.go
package utils

import (
	"crypto/sha1"
	"encoding/hex"
)

func HashURL(url string) string {
	h := sha1.New()
	h.Write([]byte(url))
	return hex.EncodeToString(h.Sum(nil))
}

/internal/ai/gemini_client.go
package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
)

type GeminiResponse struct {
	Text string `json:"text"`
}

func GenerateContent(prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	body, _ := json.Marshal(map[string]string{"prompt": prompt})

	req, _ := http.NewRequest("POST", "https://api.gemini.com/v1/generate", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Text, nil
}

/internal/ai/content_generator.go
package ai

import (
	"encoding/json"
	"fmt"
	"yourapp/internal/models"
)

func BuildPrompt(item models.FeedItemTR) string {
	return fmt.Sprintf(`
You are an expert journalist and SEO writer.

Transform the following Turkish news into an English professional article.

Include:
- SEO title (max 70 chars)
- SEO description (max 160 chars)
- 3 TLDR bullet points
- Markdown formatted content
- Suggested category
- 5 SEO-friendly tags
- Image title and description

---
Title: %s
Content: %s
Category: %s
`, item.Title, item.Content, item.Category)
}

func GenerateENFeedItem(tr models.FeedItemTR) (*models.FeedItemEN, error) {
	prompt := BuildPrompt(tr)
	response, err := GenerateContent(prompt)
	if err != nil {
		return nil, err
	}

	var en models.FeedItemEN
	if err := json.Unmarshal([]byte(response), &en); err != nil {
		return nil, err
	}
	en.GUID = tr.GUID
	return &en, nil
}

/internal/processor/feed_processor.go
package processor

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"yourapp/internal/ai"
	"yourapp/internal/models"
	"yourapp/internal/utils"
	"yourapp/internal/storage"
)

func ProcessFeeds(trFilePath string) error {
	data, _ := ioutil.ReadFile(trFilePath)
	var trItems []models.FeedItemTR
	_ = json.Unmarshal(data, &trItems)

	var enItems []models.FeedItemEN
	for _, item := range trItems {
		hash := utils.HashURL(item.URL)
		item.GUID = hash

		enItem, err := ai.GenerateENFeedItem(item)
		if err != nil {
			continue
		}
		enItem.ID = utils.HashURL(item.URL + "_en")
		enItems = append(enItems, *enItem)
	}

	filename := filepath.Join("data", "en", time.Now().Format("2006-01-02")+".json")
	return storage.SaveJSON(filename, enItems)
}

/internal/storage/file_writer.go
package storage

import (
	"encoding/json"
	"os"
)

func SaveJSON(filename string, data interface{}) error {
	os.MkdirAll("data/en", 0755)
	file, _ := os.Create(filename)
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

/cmd/process/main.go
package main

import (
	"fmt"
	"log"
	"yourapp/internal/processor"
)

func main() {
	err := processor.ProcessFeeds("data/tr/today.json")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Feed processing completed successfully.")
}

📦 Libraries & Tools
Purpose	Library
HTTP requests	net/http
JSON	encoding/json
Hashing	crypto/sha1
Env management	os
File I/O	os, ioutil
Config	spf13/viper (optional)

_____

O zaman şimdi Phase C: Output Serving & Caching Layer aşamasına geçelim.
Bu aşamada hedefimiz:

AI tarafından üretilen İngilizce JSON’ların Next.js tarafından hızlı ve güvenli biçimde alınması, gerekirse cache’lenmesi, böylece hem performansın hem de maliyetin optimize edilmesi.

🧭 Genel Mimari Akış
[Go Service] ---> [Redis Cache] ---> [Cloudflare Worker API] ---> [Next.js Client]
          ↳ writes daily_en.json


Go servisi İngilizce JSON’ları üretir ve data/en/2025-10-22.json olarak yazar.

Bu veriler Redis’e cache edilir (her item için hash key).

Cloudflare Worker bir “read API” sağlar:

Eğer istenen item Redis’te varsa direkt döner.

Yoksa JSON dosyasından okur, Redis’e yazar, sonra response döner.

Next.js app sadece Cloudflare Worker endpoint’ini çağırır — yani Go servisine veya dosya sistemine erişmez.

⚙️ Folder & File Structure
/cloudflare
  /worker
    index.js           # Worker entry
    redis.js           # Redis connection (Upstash / Cloudflare KV)
    handler.js         # Fetch & cache logic
/internal
  /storage
    redis_writer.go    # Writes EN items to Redis after generation

🧩 A. Go tarafında Redis yazma
/internal/storage/redis_writer.go
package storage

import (
	"context"
	"encoding/json"
	"os"

	"github.com/redis/go-redis/v9"
	"yourapp/internal/models"
)

var ctx = context.Background()

func NewRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
}

func SaveToRedis(items []models.FeedItemEN) error {
	rdb := NewRedisClient()
	for _, item := range items {
		data, _ := json.Marshal(item)
		rdb.Set(ctx, "feed:"+item.ID, data, 0)
	}
	return nil
}


👉 ProcessFeeds() içinde JSON yazıldıktan sonra storage.SaveToRedis(enItems) çağrısı eklenir.

🧩 B. Cloudflare Worker
/cloudflare/worker/index.js
import { getFeedItem } from './handler.js'

export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    const id = url.searchParams.get("id")
    if (!id) return new Response("Missing id", { status: 400 })

    const data = await getFeedItem(id, env)
    return new Response(JSON.stringify(data, null, 2), {
      headers: { "Content-Type": "application/json" }
    })
  }
}

/cloudflare/worker/redis.js
export async function getRedisValue(env, key) {
  const value = await env.FEED_CACHE.get(key)
  return value ? JSON.parse(value) : null
}

export async function setRedisValue(env, key, data) {
  await env.FEED_CACHE.put(key, JSON.stringify(data))
}


(Cloudflare KV veya Upstash Redis binding kullanılabilir.)

/cloudflare/worker/handler.js
import { getRedisValue, setRedisValue } from './redis.js'

export async function getFeedItem(id, env) {
  const cacheKey = `feed:${id}`
  let cached = await getRedisValue(env, cacheKey)
  if (cached) return cached

  // Fallback to JSON file URL (static asset)
  const resp = await fetch(`${env.FEED_BASE_URL}/data/en/latest.json`)
  const feeds = await resp.json()
  const item = feeds.find(f => f.id === id)
  if (!item) return { error: "Not found" }

  await setRedisValue(env, cacheKey, item)
  return item
}

🌐 Cloudflare Worker Bindings Config

wrangler.toml:

name = "feed-cache-worker"
main = "worker/index.js"
compatibility_date = "2025-10-01"

[vars]
FEED_BASE_URL = "https://cdn.example.com"

[[kv_namespaces]]
binding = "FEED_CACHE"
id = "your_kv_namespace_id"

🧱 C. Next.js Tarafı
Fetch Function (lib/getFeedItem.ts)
export async function getFeedItem(id: string) {
  const res = await fetch(`${process.env.NEXT_PUBLIC_FEED_API}/?id=${id}`, {
    next: { revalidate: 60 } // ISR-friendly
  })
  if (!res.ok) throw new Error("Feed fetch failed")
  return res.json()
}

Usage Example
const item = await getFeedItem(params.id)
return <ArticleView item={item} />

📦 Dependencies
Purpose	Go	Worker / JS
Redis client	github.com/redis/go-redis/v9	Cloudflare KV / Upstash
JSON ops	built-in	built-in
Env vars	os	Worker bindings
HTTP	built-in	fetch
Config	optional (viper)	wrangler.toml
✅ Summary
Layer	Task	Cache	Output
Go Service	TR → EN AI generation	Writes to Redis	JSON + Redis set
Cloudflare Worker	Serves data	KV / Redis	Cached API
Next.js	Fetch & render	ISR 60 s	Static page + revalidate

-------

Şimdi geldik sistemin en kritik kısmına:

AI Prompt Orchestration & Processing Layer

Burada amaç: her Türkçe haber item’ı için verimli, kaliteli ve SEO uyumlu İngilizce haber üretimi yapmak — bunu da maliyet, hız ve tekrar işlem (duplicate) riskini minimize ederek yapmak.

🧩 Amaç

Her feed item için:

AI prompt hazırlanır (kategoriye, içeriğe göre).

Gemini API çağrısı yapılır.

Dönüş kontrol edilir → içerik parse edilir.

Başarılı sonuç Redis’e ve JSON’a kaydedilir.

⚙️ Mimarinin Yeri
/internal
  /ai
    /prompts
      base.go
      seo.go
      news.go
    client.go
    processor.go

🧠 1. Prompt Strategy

Kural: her kategori (ör. politics, tech, culture, sports) için farklı ton & yapı kullanılır.
Gemini’ye “multi-instruction” yapısıyla composite bir prompt gönderilir.

/internal/ai/prompts/news.go
package prompts

import (
	"fmt"
	"strings"
)

func BuildNewsPrompt(title, content, category string) string {
	base := `
You are a professional English news editor and SEO expert.
Rewrite the following Turkish news article into a high-quality English version optimized for online publishing.

The output must include:
1. "seo_title" — short, attention-grabbing title under 70 characters.
2. "seo_description" — concise meta description under 160 characters.
3. "tldr" — three bullet points summarizing the story.
4. "content_markdown" — markdown formatted news body (professional tone, fluent English).
5. "category" — best-fit category in English.
6. "tags" — 5-7 relevant SEO keywords.
7. "image_title" and "image_description" (context-aware captions).

Respond strictly in JSON format.
`

	return fmt.Sprintf("%s\n\nOriginal Article:\nTitle: %s\n\nContent:\n%s\n\nCategory: %s",
		strings.TrimSpace(base), title, content, category)
}


Bu şekilde her haber “tek bir JSON output” dönecek şekilde promptlanır.
Bu, Next.js tarafında parse etmeyi kolaylaştırır.

⚙️ 2. Gemini Client Setup
/internal/ai/client.go
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type GeminiClient struct {
	APIKey string
	BaseURL string
	Client  *http.Client
}

func NewGeminiClient() *GeminiClient {
	return &GeminiClient{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent",
		Client:  &http.Client{Timeout: 45 * time.Second},
	}
}

type PromptInput struct {
	Contents []map[string]string `json:"contents"`
}

func (g *GeminiClient) Generate(content string) (string, error) {
	body := PromptInput{
		Contents: []map[string]string{
			{"role": "user", "parts": content},
		},
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s?key=%s", g.BaseURL, g.APIKey), bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini error: %s", string(b))
	}

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

⚙️ 3. AI Processing Logic
/internal/ai/processor.go
package ai

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"ai-news-processor/internal/ai/prompts"
	"ai-news-processor/internal/models"
	"ai-news-processor/internal/storage"
)

type Processor struct {
	Client *GeminiClient
}

func NewProcessor() *Processor {
	return &Processor{Client: NewGeminiClient()}
}

func (p *Processor) ProcessFeedItem(ctx context.Context, item models.FeedItem) (*models.FeedItemEN, error) {
	start := time.Now()

	prompt := prompts.BuildNewsPrompt(item.Title, item.Content, item.Category)
	output, err := p.Client.Generate(prompt)
	if err != nil {
		return nil, err
	}

	var parsed models.FeedItemEN
	err = json.Unmarshal([]byte(output), &parsed)
	if err != nil {
		log.Printf("[AI] Parse error: %v\nOutput: %s\n", err, output)
		return nil, err
	}

	parsed.SourceID = item.GUID
	parsed.SourceURL = item.URL
	parsed.PublishedAt = item.PublishedAt
	parsed.AIModel = "Gemini-Pro"
	parsed.ProcessedAt = time.Now()

	// Save to Redis cache
	go storage.SaveToRedis([]models.FeedItemEN{parsed})

	elapsed := time.Since(start)
	log.Printf("[AI] Processed item in %v: %s\n", elapsed, parsed.SeoTitle)
	return &parsed, nil
}

🧩 4. Model Definitions
/internal/models/news.go
package models

import "time"

type FeedItem struct {
	GUID         string    `json:"guid"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Category     string    `json:"category"`
	URL          string    `json:"url"`
	ImageURL     string    `json:"image"`
	PublishedAt  time.Time `json:"published_at"`
}

type FeedItemEN struct {
	ID              string    `json:"id"`
	SeoTitle        string    `json:"seo_title"`
	SeoDescription  string    `json:"seo_description"`
	TLDR            []string  `json:"tldr"`
	ContentMarkdown string    `json:"content_markdown"`
	Category        string    `json:"category"`
	Tags            []string  `json:"tags"`
	ImageTitle      string    `json:"image_title"`
	ImageDesc       string    `json:"image_description"`
	SourceID        string    `json:"source_id"`
	SourceURL       string    `json:"source_url"`
	AIModel         string    `json:"ai_model"`
	ProcessedAt     time.Time `json:"processed_at"`
	PublishedAt     time.Time `json:"published_at"`
}

🚀 5. Parallel Batch Processing

Feed dosyasında 100 item varsa, bunları 100 ayrı Gemini çağrısı yapmak yerine 5’lik batch’ler halinde paralel yürütüyoruz.

/internal/feed/processor.go
package feed

import (
	"context"
	"log"
	"sync"

	"ai-news-processor/internal/ai"
	"ai-news-processor/internal/models"
)

func ProcessFeeds(ctx context.Context, items []models.FeedItem) error {
	proc := ai.NewProcessor()
	batchSize := 5
	sem := make(chan struct{}, batchSize)
	var wg sync.WaitGroup

	for _, item := range items {
		sem <- struct{}{}
		wg.Add(1)

		go func(it models.FeedItem) {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := proc.ProcessFeedItem(ctx, it)
			if err != nil {
				log.Printf("[AI ERROR] %s: %v\n", it.Title, err)
			}
		}(item)
	}

	wg.Wait()
	return nil
}

🧩 6. Error Recovery

Her başarısız işlenme error_logs.json’a kaydedilebilir.

Retry mekanizması için failed GUID’ler Redis set’ine yazılır.

Sonraki cron çalışmasında tekrar işlenebilir.

🧱 Teknoloji Özeti
Amaç	Kütüphane	Açıklama
AI API	Gemini REST	Doğrudan HTTP client
Prompt templates	native	promptlar modüler ve kategori bazlı
Cache	Redis	İşlenmiş haberlerin saklanması
Parallel tasks	goroutine + semaphore	5 concurrent AI calls
Log	built-in log	izlenebilir süreç
Parse	encoding/json	Gemini’den gelen JSON kontrolü

_____

Şimdi Phase D1 – Error & Retry Management Layer’ı tasarlayacağız.

Bu aşama sistemin “sigortası” olacak — çünkü AI işlemleri (Gemini) her zaman istikrarlı çalışmaz.
Amaç:

Hatalı işlenen haberleri kaybetmeden yönetmek

Gerektiğinde otomatik yeniden işlemek (retry)

Her şeyin loglanabilir, izlenebilir olması

🧩 Genel Strateji

Her işlem üç sonuçtan birine girer:

Durum	Açıklama	Sonraki Adım
✅ Success	AI düzgün JSON döndü	Redis + JSON’a kaydedilir
⚠️ Recoverable Error	API hatası, timeout, parse hatası	Retry kuyruğuna eklenir
❌ Fatal Error	Girdi bozuk veya geçersiz	“dead-letter” (manuel inceleme) dosyasına kaydedilir
⚙️ Dosya Yapısı
/internal
  /ai
    processor.go
  /retry
    manager.go
    queue.go
    errors.go
/data
  failed/
    retry_queue.json
    dead_letter.json
  logs/
    ai_errors.log

🧠 1️⃣ Error Type Definitions
/internal/retry/errors.go
package retry

import "errors"

var (
	ErrAIResponse   = errors.New("invalid AI response format")
	ErrTimeout      = errors.New("AI request timeout")
	ErrAPILimit     = errors.New("AI API rate limit reached")
	ErrFatalContent = errors.New("fatal input content")
)

🧩 2️⃣ Retry Queue (JSON-based lightweight queue)
/internal/retry/queue.go
package retry

import (
	"encoding/json"
	"os"
	"sync"
)

var queueLock sync.Mutex

type RetryItem struct {
	GUID     string `json:"guid"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
}

const retryFile = "data/failed/retry_queue.json"
const deadLetterFile = "data/failed/dead_letter.json"

func loadQueue() ([]RetryItem, error) {
	data, err := os.ReadFile(retryFile)
	if err != nil {
		return []RetryItem{}, nil
	}
	var q []RetryItem
	_ = json.Unmarshal(data, &q)
	return q, nil
}

func saveQueue(q []RetryItem) error {
	data, _ := json.MarshalIndent(q, "", "  ")
	return os.WriteFile(retryFile, data, 0644)
}

func AddToRetryQueue(item RetryItem) error {
	queueLock.Lock()
	defer queueLock.Unlock()
	q, _ := loadQueue()
	q = append(q, item)
	return saveQueue(q)
}

func MoveToDeadLetter(item RetryItem) error {
	data, _ := json.MarshalIndent(item, "", "  ")
	f, _ := os.OpenFile(deadLetterFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
	return nil
}

⚙️ 3️⃣ Retry Manager Logic
/internal/retry/manager.go
package retry

import (
	"context"
	"log"
	"time"

	"ai-news-processor/internal/ai"
	"ai-news-processor/internal/models"
)

func RetryFailedItems(ctx context.Context) {
	items, _ := loadQueue()
	if len(items) == 0 {
		log.Println("[RETRY] No failed items to process.")
		return
	}

	proc := ai.NewProcessor()
	successful := []int{}

	for i, it := range items {
		if it.Attempts >= 3 {
			log.Printf("[RETRY] Moving %s to dead letter after 3 attempts.\n", it.GUID)
			MoveToDeadLetter(it)
			continue
		}

		time.Sleep(2 * time.Second) // Small delay to prevent rate limits

		feed := models.FeedItem{
			GUID:     it.GUID,
			Title:    it.Title,
			Content:  it.Content,
			Category: it.Category,
		}

		_, err := proc.ProcessFeedItem(ctx, feed)
		if err != nil {
			log.Printf("[RETRY] Failed again: %s (%v)\n", it.Title, err)
			it.Attempts++
			AddToRetryQueue(it)
		} else {
			successful = append(successful, i)
			log.Printf("[RETRY] ✅ Successfully reprocessed %s\n", it.Title)
		}
	}

	log.Printf("[RETRY] Completed. Success: %d / Total: %d\n", len(successful), len(items))
}

⚙️ 4️⃣ Processor İçinde Entegrasyon
/internal/ai/processor.go — ilgili kısmı güncelle
import "ai-news-processor/internal/retry"

// ...

func (p *Processor) ProcessFeedItem(ctx context.Context, item models.FeedItem) (*models.FeedItemEN, error) {
	start := time.Now()

	prompt := prompts.BuildNewsPrompt(item.Title, item.Content, item.Category)
	output, err := p.Client.Generate(prompt)
	if err != nil {
		retry.AddToRetryQueue(retry.RetryItem{
			GUID:     item.GUID,
			Title:    item.Title,
			Content:  item.Content,
			Category: item.Category,
			Error:    err.Error(),
			Attempts: 1,
		})
		return nil, err
	}

	var parsed models.FeedItemEN
	err = json.Unmarshal([]byte(output), &parsed)
	if err != nil {
		retry.AddToRetryQueue(retry.RetryItem{
			GUID:     item.GUID,
			Title:    item.Title,
			Content:  item.Content,
			Category: item.Category,
			Error:    "parse_error",
			Attempts: 1,
		})
		return nil, err
	}

	// ... success case devam
}

🕓 5️⃣ Scheduler’da Retry Entegrasyonu
/internal/scheduler/scheduler.go
_, _ = c.AddFunc("0 */2 * * * *", func() {
	log.Println("[RETRY] Running retry manager...")
	retry.RetryFailedItems(context.Background())
})


➡️ Bu cron her 2 saatte bir retry kuyruğunu çalıştırır.
Her item en fazla 3 kez denenir.

📋 6️⃣ Loglama

Basit ama güçlü bir log yapısı:

/internal/logging/logger.go
package logging

import (
	"log"
	"os"
)

func Init() {
	file, err := os.OpenFile("data/logs/ai_errors.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	log.SetOutput(file)
	log.Println("=== Logger initialized ===")
}


main.go içinde:

logging.Init()

✅ 7️⃣ Özellik Özeti
Özellik	Açıklama
Queue	JSON dosyası, disk tabanlı
Retry limit	3 deneme
Delay	2 s rate limit koruması
Dead-letter	Manuel inceleme için kayıt
Log	data/logs/ai_errors.log
Cron	Her 2 saatte bir otomatik retry

_____

Harika, o zaman D1: /internal/ai/pipeline.go dosyasının örnek kodlarını yazalım.
Bu dosya, Türkçe feed item’larını alıp AI pipeline’ından geçirerek profesyonel İngilizce haber verilerini üretir.
Hedefimiz: Temiz, extensible, production-level bir pipeline yapısı kurmak.

📁 /internal/ai/pipeline.go
package ai

import (
	"context"
	"fmt"
	"log"
	"strings"

	"your_project/internal/types"
	"your_project/internal/utils"
)

// AIPipeline orchestrates the AI processing steps for one feed item
type AIPipeline struct {
	client AIClient
}

func NewAIPipeline(client AIClient) *AIPipeline {
	return &AIPipeline{client: client}
}

// ProcessFeedItem converts a Turkish feed item into an English news item
func (p *AIPipeline) ProcessFeedItem(ctx context.Context, item types.FeedItem) (*types.NewsItem, error) {
	// 1. Deduplicate check (hash of URL)
	hash := utils.GenerateHash(item.URL)
	if utils.IsDuplicate(hash) {
		log.Printf("Skipping duplicate feed item: %s", item.URL)
		return nil, nil
	}

	// 2. Prepare AI prompt
	prompt := buildPrompt(item)

	// 3. Call AI model (Gemini / OpenAI etc.)
	resp, err := p.client.GenerateNewsContent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %w", err)
	}

	// 4. Parse AI structured output
	news, err := parseAIResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("AI response parsing failed: %w", err)
	}

	// 5. Assign IDs and metadata
	news.SourceURL = item.URL
	news.SourceHash = hash
	news.OriginalLang = "tr"
	news.TranslatedLang = "en"
	news.CreatedAt = utils.NowISO()

	return news, nil
}

func buildPrompt(item types.FeedItem) string {
	return fmt.Sprintf(`
You are a professional journalist and SEO expert.

Transform the following Turkish news into a well-written English article optimized for SEO.
Provide structured JSON output with:
- seo_title (max 60 chars)
- seo_description (max 160 chars)
- tldr_bullets (3 concise bullet points)
- markdown_content
- category
- tags (5 relevant words)
- image_title
- image_description

Turkish Source:
Title: %s
Content: %s
Image: %s
URL: %s
`, item.Title, strings.TrimSpace(item.Content), item.Image, item.URL)
}

func parseAIResponse(resp string) (*types.NewsItem, error) {
	// You may use json.Unmarshal if the AI outputs structured JSON.
	// For now, mock a simplified parser.
	news := &types.NewsItem{
		TitleEN:       utils.ExtractJSONValue(resp, "seo_title"),
		SEODescription: utils.ExtractJSONValue(resp, "seo_description"),
		ContentEN:     utils.ExtractJSONValue(resp, "markdown_content"),
		Category:      utils.ExtractJSONValue(resp, "category"),
		Tags:          utils.ExtractJSONList(resp, "tags"),
		ImageTitle:    utils.ExtractJSONValue(resp, "image_title"),
		ImageDesc:     utils.ExtractJSONValue(resp, "image_description"),
	}
	return news, nil
}

📁 /internal/ai/client.go

(AI Client interface ve Gemini implementation)

package ai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type AIClient interface {
	GenerateNewsContent(ctx context.Context, prompt string) (string, error)
}

type GeminiClient struct {
	APIKey string
}

func NewGeminiClient(apiKey string) *GeminiClient {
	return &GeminiClient{APIKey: apiKey}
}

func (c *GeminiClient) GenerateNewsContent(ctx context.Context, prompt string) (string, error) {
	reqBody := fmt.Sprintf(`{"model":"gemini-pro","prompt":%q}`, prompt)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://generativelanguage.googleapis.com/v1/models/gemini-pro:generateText?key="+c.APIKey, strings.NewReader(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

🧠 Ne Sağladık?

✅ Feed item’ların tekrar işlenmemesini sağladık (hash ile kontrol).
✅ AI’ye temiz, net bir prompt formatı verdik (habercilik + SEO odaklı).
✅ AI output’unu modüler parse ettik.
✅ Pipeline, bağımsız bir Go servis olarak API üzerinden Next.js tarafından çağrılabilir.

_____

O halde bir sonraki adım — D2: /cmd/worker/main.go ile devam ediyoruz.

Bu dosya pipeline’ı tetikleyen arka plan worker’dır:

Feed listesini okur (örneğin /feeds klasöründen veya bir feeds.json dosyasından).

Her feed’in URL’sini fetch eder.

Yeni feed item’larını alır, Redis veya bir lokal cache üzerinden “işlendi mi?” kontrolü yapar.

İşlenmeyenleri AI Pipeline’a gönderir.

Ortaya çıkan İngilizce haber JSON’larını /data/news/YYYY-MM-DD/ dizinine kaydeder.

📁 cmd/worker/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"your_project/internal/ai"
	"your_project/internal/feed"
	"your_project/internal/types"
	"your_project/internal/utils"
)

func main() {
	ctx := context.Background()
	log.Println("🚀 Worker started...")

	// 1. Load feeds
	feeds, err := loadFeeds("data/feeds.json")
	if err != nil {
		log.Fatalf("Failed to load feeds: %v", err)
	}

	// 2. Initialize AI pipeline
	gemini := ai.NewGeminiClient(os.Getenv("GEMINI_API_KEY"))
	pipeline := ai.NewAIPipeline(gemini)

	// 3. Iterate feeds
	for _, f := range feeds {
		log.Printf("Fetching feed: %s", f.Name)
		items, err := feed.FetchFeedItems(ctx, f.URL)
		if err != nil {
			log.Printf("❌ Error fetching feed %s: %v", f.URL, err)
			continue
		}

		// 4. Process each item
		for _, item := range items {
			news, err := pipeline.ProcessFeedItem(ctx, item)
			if err != nil {
				log.Printf("⚠️ AI processing failed for %s: %v", item.URL, err)
				continue
			}
			if news == nil {
				continue // duplicate skip
			}

			// 5. Save generated news JSON
			if err := saveNewsJSON(news); err != nil {
				log.Printf("❌ Failed saving news: %v", err)
			}
		}
	}

	log.Println("✅ Worker finished successfully.")
}

// Load feed definitions
func loadFeeds(path string) ([]types.Feed, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var feeds []types.Feed
	if err := json.Unmarshal(file, &feeds); err != nil {
		return nil, err
	}

	return feeds, nil
}

// Save news as daily JSON files grouped by date
func saveNewsJSON(news *types.NewsItem) error {
	dateDir := filepath.Join("data", "news", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return err
	}

	fileName := fmt.Sprintf("%s.json", utils.Slugify(news.TitleEN))
	filePath := filepath.Join(dateDir, fileName)

	data, err := json.MarshalIndent(news, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

📁 internal/feed/fetch.go

Bu modül, feed kaynağını (örneğin Türkçe JSON API) okur ve []FeedItem döner.
Örnek olarak bir JSON feed endpoint’ini fetch eden yapı:

package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"your_project/internal/types"
)

func FetchFeedItems(ctx context.Context, feedURL string) ([]types.FeedItem, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed fetch failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []types.FeedItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}

	return items, nil
}

🔧 Özet: Worker’in Akış Diyagramı
Load feeds.json
      ↓
Fetch each feed URL
      ↓
For each feed item:
   ├─> Check duplicate (Redis/Hash)
   ├─> Build AI prompt
   ├─> Call Gemini API
   ├─> Parse structured response
   └─> Save news as English JSON

🧠 Devam Planı

Şimdi sistemimizde şu iki önemli eksik kaldı:

Redis cache veya local deduplication layer

Hangi item işlendi (hash listesi) → tekrar işlemeyi önleyecek.

Next.js entegrasyonu

Worker’ın ürettiği JSON’ları (örneğin /data/news/YYYY-MM-DD/*.json) okuyacak API routes veya cron job’lar.

	______

Redis'i halledelim. Nextjs'yi ayrı bir proje olarak ele alalım. Sadece golang project ve onun ihtiyaçlarına odaklanalim