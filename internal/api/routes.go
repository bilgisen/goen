package api

import (
	"log"

	"github.com/bilgisen/goen/internal/cache"
	"github.com/bilgisen/goen/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// SetupRoutes configures all the routes for the application
func SetupRoutes(app *fiber.App, redisClient *cache.RedisClient, cfg *config.Config) {
	// Initialize handlers
	handlers, err := NewHandlers(cfg, redisClient)
	if err != nil {
		log.Fatalf("Failed to initialize handlers: %v", err)
	}

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${method} ${path} - ${status} - ${latency}\n",
	}))

	// API group with versioning
	api := app.Group("/api/v1")

	// Health check endpoint
	api.Get("/health", handlers.HealthCheck)

	// News endpoints
	news := api.Group("/news")
	{
		news.Get("", handlers.GetNews)           // List news with pagination
		news.Get("/:id", handlers.GetNewsByID)    // Get single news by ID
	}

	// Admin endpoints (protected in production)
	admin := api.Group("/admin")
	{
		admin.Post("/process", handlers.ProcessFeeds) // Process new feeds
		admin.Delete("/news/:id", handlers.DeleteNews) // Delete a news item
	}

	// 404 Handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Endpoint not found",
		})
	})

	// Error handler
	app.Use(func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			// Default to 500 status code
			code := fiber.StatusInternalServerError
			e, ok := err.(*fiber.Error)
			if ok {
				code = e.Code
			}

			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return nil
	})
}
