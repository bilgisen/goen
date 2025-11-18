package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bilgisen/goen/internal/ai"
	"github.com/bilgisen/goen/internal/cache"
	"github.com/bilgisen/goen/internal/config"
	"github.com/bilgisen/goen/internal/feed"
	"github.com/bilgisen/goen/internal/logger"
	"github.com/bilgisen/goen/internal/models"
	"github.com/bilgisen/goen/internal/storage"
	"github.com/gofiber/fiber/v2"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type Handlers struct {
	config    *config.Config
	redis     cache.RedisInterface
	storage   storage.Storage
	processor *feed.Processor
	gemini    *ai.GeminiClient
	postProc  *ai.PostProcessor
}

func NewHandlers(cfg *config.Config, redis cache.RedisInterface) (*Handlers, error) {
	// Initialize storage based on configuration
	var storageImpl storage.Storage
	var err error

	// Use R2 storage if R2 credentials are configured
	if cfg.R2Endpoint != "" && cfg.R2AccessKey != "" && cfg.R2SecretKey != "" && cfg.R2Bucket != "" {
		logger.Get().Info().
			Str("r2_endpoint", cfg.R2Endpoint).
			Str("r2_bucket", cfg.R2Bucket).
			Msg("R2 credentials found, initializing R2 storage")
		storageImpl, err = storage.NewR2Storage(cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket, cfg.R2AccountID)
		if err != nil {
			logger.Get().Error().
				Err(err).
				Msg("Failed to initialize R2 storage")
			return nil, fmt.Errorf("failed to initialize R2 storage: %w", err)
		}
		logger.Get().Info().Msg("R2 storage initialized successfully")
	} else {
		logger.Get().Info().
			Str("storage_path", cfg.ProcessedPath).
			Msg("R2 credentials not found, using file storage")
		storageImpl, err = storage.NewFileStorage(cfg.ProcessedPath)
		if err != nil {
			logger.Get().Error().
				Err(err).
				Msg("Failed to initialize file storage")
			return nil, fmt.Errorf("failed to initialize file storage: %w", err)
		}
	}

	// Initialize Gemini client with Redis-based rate limiting
	var gemini *ai.GeminiClient
	if cfg.AIApiKey != "" && cfg.AIApiKey != "test-key" {
		var err error
		gemini, err = ai.NewGeminiClient(
			cfg.AIApiKey,
			cfg.AIModel,
			cfg.AIRateLimit,  // Requests per minute
			cfg.AITPMLimit,   // Tokens per minute
			cfg.RedisURL,     // Redis connection URL
		)
		if err != nil {
			logger.Get().Error().
				Err(err).
				Msg("Failed to initialize Gemini client with rate limiting")
		} else {
			logger.Get().Info().
				Int("rpm", cfg.AIRateLimit).
				Int("tpm", cfg.AITPMLimit).
				Msg("Initialized Gemini client with Redis rate limiting")
		}
	}

	return &Handlers{
		config:    cfg,
		redis:     redis,
		storage:   storageImpl,
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
		pageSize = 10
	}

	// Create a context with timeout (increase to 30s due to R2 listing/fetch latency)
	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()

	// Create channels for the result
	resultChan := make(chan []*models.NewsItem, 1)
	errChan := make(chan error, 1)

	// Run the storage operation in a goroutine
	go func() {
		items, err := h.storage.ListNews(ctx, page, pageSize)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- items
	}()

	// Wait for either the result, an error, or a timeout
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
				"error": "Request timed out",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Request cancelled",
		})
	case err := <-errChan:
		if err != nil {
			// On storage error, return an empty list instead of 500 to keep API responsive
			logger.Get().Warn().Err(err).Msg("ListNews error - returning empty list")
			return c.JSON(fiber.Map{
				"page":       page,
				"page_size":  pageSize,
				"total":      0,
				"items":      []*models.NewsItem{},
			})
		}
	case news := <-resultChan:
		// Ensure featured is never nil for API response
		for _, it := range news {
			if it != nil && it.Featured == nil {
				def := false
				it.Featured = &def
			}
		}
		return c.JSON(fiber.Map{
			"page":       page,
			"page_size":  pageSize,
			"total":      len(news),
			"items":      news,
		})
	}

	return nil
}

// GetNewsByID handles GET /api/news/:id
func (h *Handlers) GetNewsByID(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "News ID is required",
		})
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()

	// Create channels for the result
	resultChan := make(chan *models.NewsItem, 1)
	errChan := make(chan error, 1)

	// Run the storage operation in a goroutine
	go func() {
		item, err := h.storage.GetNewsByID(ctx, id)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- item
	}()

	// Wait for either the result, an error, or a timeout
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
				"error": "Request timed out",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Request cancelled",
		})
	case err := <-errChan:
		if err != nil {
			logger.Get().Error().Err(err).Str("id", id).Msg("Error getting news item")
			if strings.Contains(err.Error(), "not found") {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "News not found",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to get news item",
			})
		}
	case item := <-resultChan:
		return c.JSON(item)
	}

	return nil
}

// ProcessFeedsExternal handles POST /api/v1/external/process
// This is a simplified version of ProcessFeeds for external/cron use
// API key is optional but recommended for production use
// This endpoint processes feeds asynchronously and returns immediately
func (h *Handlers) ProcessFeedsExternal(c *fiber.Ctx) error {
	// Check API key if configured
	if h.config.ExternalApiKey != "" {
		apiKey := c.Get("X-API-Key")
		if apiKey != h.config.ExternalApiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"status":  "error",
				"message": "Invalid or missing API key",
			})
		}
	}

	// Get feed URLs from request body or use default from config
	var requestBody struct {
		FeedURLs []string `json:"feed_urls"`
	}

	if err := c.BodyParser(&requestBody); err != nil {
		// If no body provided, use default feeds from config
		requestBody.FeedURLs = h.config.FeedURLs
	}

	// If still no feed URLs, return error
	if len(requestBody.FeedURLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "No feed URLs provided and no default feeds configured",
		})
	}

	log := logger.Get()
	log.Info().
		Strs("feed_urls", requestBody.FeedURLs).
		Msg("Starting external feed processing")

	// Start processing in a goroutine
	go func(urls []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		log.Info().
			Int("feed_count", len(urls)).
			Msg("Starting feed processing in background")

		// Check for required services
		if h.gemini == nil || h.postProc == nil {
			log.Error().
				Bool("gemini_initialized", h.gemini != nil).
				Bool("postproc_initialized", h.postProc != nil).
				Msg("Required services not initialized")
			return
		}

		// Process feeds
		items, err := h.processor.ProcessFeeds(ctx, urls)
		if err != nil {
			log.Error().
				Err(err).
				Int("url_count", len(urls)).
				Msg("Error processing feeds")
			return
		}

		log.Info().
			Int("items_to_process", len(items)).
			Msg("Starting to process feed items with AI")

		// Process each item with AI
		for _, item := range items {
			select {
			case <-ctx.Done():
				log.Warn().
					Str("guid", item.Guid).
					Msg("Processing cancelled due to timeout")
				return
			default:
				// Skip AI processing if Gemini client is not available
				if h.gemini == nil {
					log.Warn().
						Str("title", item.TitleTR).
						Msg("Gemini client not available, skipping AI processing")
					continue
				}

				// Generate English content using Gemini
				newsItem, err := h.gemini.GenerateEnglishNews(ctx, item)
				if err != nil {
					log.Error().
						Err(err).
						Str("title", item.TitleTR).
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

				// Save the processed item to storage
				if err := h.storage.SaveNews(ctx, newsItem); err != nil {
					log.Error().
						Err(err).
						Str("id", newsItem.ID).
						Msg("Error saving news item to storage")
				} else {
					log.Info().
						Str("id", newsItem.ID).
						Msg("Successfully saved news item to storage")
				}

				// Mark as processed
				if err := h.processor.MarkAsProcessed(ctx, []string{item.Guid}, h.config.CacheTTL); err != nil {
					log.Error().
						Err(err).
						Str("guid", item.Guid).
						Msg("Error marking item as processed")
				}
			}
		}

		log.Info().
			Int("total_items_processed", len(items)).
			Msg("Finished processing all feed items")

	}(requestBody.FeedURLs)

	// Return immediately with processing started message
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": fmt.Sprintf("Processing %d feed(s) in the background", len(requestBody.FeedURLs)),
		"feeds":   requestBody.FeedURLs,
	})
}

// ProcessFeeds handles POST /api/admin/process
func (h *Handlers) ProcessFeeds(c *fiber.Ctx) error {
	// Check API key for admin endpoints
	// Temporarily disabled for testing
	// if h.config.AdminAPIKey != "" {
	// 	apiKey := c.Get("X-API-Key")
	// 	if apiKey != h.config.AdminAPIKey {
	// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
	// 			"error": "Invalid API key",
	// 		})
	// 	}
	// }

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

				// Save the processed item to primary storage (R2 or file)
				if h.storage != nil {
					if err := h.storage.SaveNews(ctx, newsItem); err != nil {
						log.Error().
							Err(err).
							Str("id", newsItem.ID).
							Msg("Error saving news item to storage")
					} else {
						log.Info().
							Str("id", newsItem.ID).
							Msg("Successfully saved news item to storage")
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

// DebugListAllNews is a debug endpoint to list all news items
func (h *Handlers) DebugListAllNews(c *fiber.Ctx) error {
	log := logger.Get()
	
	// Use a large page size to get all items
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	items, err := h.storage.ListNews(ctx, 1, 1000) // Get up to 1000 items from the first page
	if err != nil {
		log.Error().
			Err(err).
			Msg("Error getting all news items")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get news items",
		})
	}

	// Get storage type for debugging
	storageType := "unknown"
	switch h.storage.(type) {
	case *storage.FileStorage:
		storageType = "file"
	case *storage.R2Storage:
		storageType = "r2"
	}

	log.Info().
		Int("total_items", len(items)).
		Str("storage_type", storageType).
		Msg("Debug: Listed all news items")

	return c.JSON(fiber.Map{
		"status":     "success",
		"count":      len(items),
		"storage":    storageType,
		"items":      items,
	})
}

// DeleteNews handles DELETE /api/admin/news/:id
func (h *Handlers) DeleteNews(c *fiber.Ctx) error {
	// Check API key for admin endpoints
	if h.config.AdminAPIKey != "" {
		apiKey := c.Get("X-API-Key")
		if apiKey != h.config.AdminAPIKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}
	}

	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "News ID is required",
		})
	}

	// Use context with timeout for delete operation
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	if err := h.storage.DeleteNews(ctx, id); err != nil {
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
