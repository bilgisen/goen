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
	redis     *cache.RedisClient
	storage   *storage.Storage
	processor *feed.Processor
	gemini    *ai.GeminiClient
	postProc  *ai.PostProcessor
}

func NewHandlers(cfg *config.Config, redis *cache.RedisClient) (*Handlers, error) {
	storage, err := storage.NewStorage(cfg.ProcessedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	return &Handlers{
		config:    cfg,
		redis:     redis,
		storage:   storage,
		processor: feed.NewProcessor(redis),
		gemini:    ai.NewGeminiClient(cfg.AIApiKey, cfg.AIModel),
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
	var req struct {
		FeedURLs []string `json:"feed_urls"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if len(req.FeedURLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least one feed URL is required",
		})
	}

	// Start processing in a goroutine
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		// Process feeds
		items, err := h.processor.ProcessFeeds(ctx, req.FeedURLs)
		if err != nil {
			logger.Get().Info().Int("url_count", len(req.FeedURLs)).Msg("Starting feed processing")
			return
		}

		// Process each item with AI
		for _, item := range items {
			select {
			case <-ctx.Done():
				logger.Get().Info().Msg("Processing cancelled")
				return
			default:
				// Generate English version using Gemini
				newsItem, err := h.gemini.GenerateEnglishNews(ctx, item)
				if err != nil {
					logger.Get().Error().Err(err).Str("url", item.Url).Msg("Error generating English news")
					continue
				}

				// Post-process the generated content
				if h.postProc != nil {
					if err := h.postProc.ProcessNewsItem(newsItem); err != nil {
						logger.Get().Error().Err(err).Str("id", newsItem.ID).Msg("Error post-processing news item")
						continue
					}
				}

				// Save to storage
				if h.storage != nil {
					if err := h.storage.SaveNews(ctx, newsItem); err != nil {
						logger.Get().Error().Err(err).Str("id", newsItem.ID).Msg("Error saving news item")
						continue
					}
				}

				// Mark as processed
				if h.processor != nil {
					if err := h.processor.MarkAsProcessed(ctx, []string{item.Url}, h.config.CacheTTL); err != nil {
						logger.Get().Error().Err(err).Str("url", item.Url).Msg("Error marking item as processed")
					}
				}
			}
		}
	}()

	return c.JSON(fiber.Map{
		"status":  "started",
		"message": "Feed processing started in the background",
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
