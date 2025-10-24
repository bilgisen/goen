package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/bilgisen/goen/internal/api"
    "github.com/bilgisen/goen/internal/cache"
    "github.com/bilgisen/goen/internal/config"
    "github.com/bilgisen/goen/internal/logger"
    "github.com/bilgisen/goen/internal/middleware"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
    // Load and validate configuration
    cfg := config.Load()

    // Initialize logger
    if err := logger.Init(logger.Config{
        Level:  cfg.LogLevel,
        Output: "stdout",
        Pretty: true,
    }); err != nil {
        panic(err)
    }

    log := logger.Get()
    log.Info().Msg("Starting application...")

    // Initialize Redis client
    var redisClient cache.RedisInterface
    redisClient, err := cache.NewRedisClient(cfg)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to initialize Redis client")
    }
    defer func() {
        log.Info().Msg("Closing Redis client...")
        if err := redisClient.Close(); err != nil {
            log.Error().Err(err).Msg("Error closing Redis client")
        }
    }()

    // Create Fiber app with custom config
    app := fiber.New(fiber.Config{
        ReadTimeout:  cfg.HTTPTimeout,
        WriteTimeout: cfg.HTTPTimeout,
        IdleTimeout:  120 * time.Second,
        ErrorHandler: middleware.ErrorHandler,
    })

    // Global middleware
    app.Use(recover.New()) // Recover from panics
    app.Use(middleware.RequestLogger())

    // Serve index.html directly
    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendFile("./web/static/index.html")
    })

    // Setup API routes
    api.SetupRoutes(app, redisClient, cfg)

    // Start server in a goroutine
    go func() {
        log.Info().Str("port", cfg.Port).Msg("Starting server")
        if err := app.Listen(":" + cfg.Port); err != nil {
            log.Fatal().Err(err).Msg("Server error")
        }
    }()

    // Wait for interrupt signal to gracefully shut down the server
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit

    log.Info().Msg("Shutting down server...")

    // Create a deadline for graceful shutdown
    ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
    defer cancel()

    // Shutdown the server
    if err := app.ShutdownWithContext(ctx); err != nil {
        log.Error().Err(err).Msg("Server forced to shutdown")
    }

    log.Info().Msg("Server exited properly")
}
