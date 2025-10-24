package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/bilgisen/goen/internal/ai"
	"github.com/bilgisen/goen/internal/cache"
	"github.com/bilgisen/goen/internal/config"
	"github.com/bilgisen/goen/internal/feed"
	"github.com/bilgisen/goen/internal/logger"
	"github.com/bilgisen/goen/internal/storage"
	"github.com/gofiber/fiber/v2"
)

type Handlers struct {
	config    *config.Config
	redis     cache.RedisInterface
	storage   *storage.Storage
	processor *feed.Processor
	gemini    *ai.GeminiClient
	postProc  *ai.PostProcessor
}

func NewHandlers(cfg *config.Config, redis cache.RedisInterface) (*Handlers, error) {
	storage, err := storage.NewStorage(cfg.ProcessedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize Gemini client (optional for basic functionality)
	var gemini *ai.GeminiClient
	if cfg.AIApiKey != "" && cfg.AIApiKey != "test-key" {
		gemini = ai.NewGeminiClient(cfg.AIApiKey, cfg.AIModel)
	}

	return &Handlers{
		config:    cfg,
		redis:     redis,
		storage:   storage,
		processor: feed.NewProcessor(redis),
		gemini:    gemini,
		postProc:  ai.NewPostProcessor(),
	}, nil
}

// HealthCheck handles the /health endpoint
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

// GetNews handles GET /api/news
func (h *Handlers) GetNews(c *fiber.Ctx) error {
	// Parse pagination parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	switch {
	case pageSize > 100:
		pageSize = 100
	case pageSize <= 0:
		pageSize = 20
	}

	// Get news from storage
	news, err := h.storage.ListNews(c.Context(), page, pageSize)
	if err != nil {
		logger.Get().Error().Err(err).Msg("Error getting news")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get news",
		})
	}

	return c.JSON(fiber.Map{
		"page":       page,
		"page_size":  pageSize,
		"total":      len(news),
		"items":      news,
	})
}

// GetNewsByID handles GET /api/news/:id
func (h *Handlers) GetNewsByID(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "News ID is required",
		})
	}

	news, err := h.storage.GetNewsByID(c.Context(), id)
	if err != nil {
		logger.Get().Error().Err(err).Str("id", id).Msg("Error getting news item")
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "News not found",
		})
	}

	return c.JSON(news)
}

// ProcessFeeds handles POST /api/admin/process
func (h *Handlers) ProcessFeeds(c *fiber.Ctx) error {
	log := logger.Get()
	start := time.Now()
	
	log.Info().
		Str("ip", c.IP()).
		Str("method", c.Method()).
		Str("path", c.Path()).
		Msg("Received process feeds request")

	var req struct {
		FeedURLs []string `json:"feed_urls"`
	}

	if err := c.BodyParser(&req); err != nil {
		log.Error().
			Err(err).
			Msg("Invalid request body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body: " + err.Error(),
		})
	}

	if len(req.FeedURLs) == 0 {
		log.Warn().Msg("No feed URLs provided")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No feed URLs provided",
		})
	}

	log.Info().
		Int("feed_count", len(req.FeedURLs)).
		Msg("Starting background processing of feeds")

	// Start processing in a goroutine
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		log.Info().
			Int("feed_count", len(req.FeedURLs)).
			Dur("timeout", 30*time.Minute).
			Msg("Starting feed processing in background")

		// Process feeds
		items, err := h.processor.ProcessFeeds(ctx, req.FeedURLs)
		if err != nil {
			log.Error().
				Err(err).
				Int("url_count", len(req.FeedURLs)).
				Msg("Error processing feeds")
			return
		}

		log.Info().
			Int("items_to_process", len(items)).
			Dur("fetch_duration", time.Since(start)).
			Msg("Starting to process feed items with AI")

		// Process each item with AI
		for i, item := range items {
			select {
			case <-ctx.Done():
				log.Warn().
					Int("processed_items", i).
					Int("total_items", len(items)).
					Msg("Processing cancelled due to timeout")
				return
			default:
				// Log progress every 5 items
				if i > 0 && i%5 == 0 {
					log.Info().
						Int("processed", i).
						Int("remaining", len(items)-i).
						Dur("elapsed", time.Since(start)).
						Msg("Processing feed items")
				}

				// Skip AI processing if Gemini client is not available
				if h.gemini == nil {
					log.Warn().
						Str("title", item.TitleTR).
						Int("item_index", i).
						Msg("Gemini client not available, skipping AI processing")
					continue
				}

				// Generate English version using Gemini
				newsItem, err := h.gemini.GenerateEnglishNews(ctx, item)
				if err != nil {
					log.Error().
						Err(err).
						Str("title", item.TitleTR).
						Int("item_index", i).
						Msg("Error generating English news")
					continue
				}

				// Post-process the generated content
				if h.postProc != nil {
					if err := h.postProc.ProcessNewsItem(newsItem); err != nil {
						log.Error().
							Err(err).
							Str("id", newsItem.ID).
							Msg("Error post-processing news item")
						continue
					}
				}

				// Save the processed item
				if h.storage != nil {
					if err := h.storage.SaveNews(ctx, newsItem); err != nil {
						log.Error().
							Err(err).
							Str("id", newsItem.ID).
							Msg("Error saving news item")
					}
				}

				// Mark as processed
				if h.processor != nil {
					if err := h.processor.MarkAsProcessed(ctx, []string{item.Guid}, h.config.CacheTTL); err != nil {
						log.Error().
							Err(err).
							Str("guid", item.Guid).
							Msg("Error marking item as processed")
					}
				}
			}
		}

		log.Info().
			Int("total_items_processed", len(items)).
			Dur("total_duration", time.Since(start)).
			Msg("Finished processing all feed items")
	}()

	log.Info().
		Dur("request_duration", time.Since(start)).
		Msg("Request processed, background job started")

	return c.JSON(fiber.Map{
		"status":  "started",
		"message": fmt.Sprintf("Processing %d feed(s) in the background", len(req.FeedURLs)),
		"feeds":   len(req.FeedURLs),
	})
}

// DeleteNews handles DELETE /api/admin/news/:id
func (h *Handlers) DeleteNews(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "News ID is required",
		})
	}

	if err := h.storage.DeleteNews(c.Context(), id); err != nil {
		logger.Get().Error().Err(err).Str("id", id).Msg("Error getting news item")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get news item",
		})
	}

	return c.JSON(fiber.Map{
		"status":  "deleted",
		"message": "News item deleted successfully",
	})
}
