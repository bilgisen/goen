package middleware

import (
    "time"

    "github.com/bilgisen/goen/internal/logger"
    "github.com/gofiber/fiber/v2"
    "github.com/rs/zerolog"
)

// LoggerConfig defines the config for the logger middleware
type LoggerConfig struct {
    // Skip defines a function to skip middleware.
    // Optional. Default: nil
    Next func(c *fiber.Ctx) bool

    // Logger is the zerolog logger instance to use.
    // If not provided, the default logger will be used.
    Logger *zerolog.Logger

    // Fields to include in the logs
    Fields []string
}

// DefaultLoggerConfig is the default config
var DefaultLoggerConfig = LoggerConfig{
    Next:   nil,
    Fields: []string{"latency", "status", "method", "path", "ip", "user_agent"},
}

// NewLogger creates a new middleware handler
func NewLogger(config ...LoggerConfig) fiber.Handler {
    // Set default config
    cfg := DefaultLoggerConfig

    // Override config if provided
    if len(config) > 0 {
        cfg = config[0]

        // Set default values
        if cfg.Next == nil {
            cfg.Next = DefaultLoggerConfig.Next
        }
        if len(cfg.Fields) == 0 {
            cfg.Fields = DefaultLoggerConfig.Fields
        }
    }

    // Set default logger if not provided
    if cfg.Logger == nil {
        log := logger.Get()
        cfg.Logger = log
    }

    // Create a set of fields for quick lookup
    fields := make(map[string]bool)
    for _, f := range cfg.Fields {
        fields[f] = true
    }

    // Return new handler
    return func(c *fiber.Ctx) error {
        // Skip middleware if Next returns true
        if cfg.Next != nil && cfg.Next(c) {
            return c.Next()
        }

        // Start timer
        start := time.Now()

        // Handle request
        err := c.Next()

        // Calculate latency
        latency := time.Since(start)

        // Create log event
        event := cfg.Logger.Info()

        // Add fields based on config
        if fields["method"] {
            event = event.Str("method", c.Method())
        }
        if fields["path"] {
            event = event.Str("path", c.Path())
        }
        if fields["status"] {
            event = event.Int("status", c.Response().StatusCode())
        }
        if fields["ip"] {
            event = event.Str("ip", c.IP())
        }
        if fields["user_agent"] {
            event = event.Str("user_agent", c.Get("User-Agent"))
        }
        if fields["latency"] {
            event = event.Dur("latency", latency)
        }

        // Add error if exists
        if err != nil {
            event = event.Err(err)
        }

        // Log the request
        event.Msg("request")

        return err
    }
}

// RequestLogger is a simpler version of the logger middleware
func RequestLogger() fiber.Handler {
    return NewLogger(LoggerConfig{
        Fields: []string{"latency", "status", "method", "path", "ip"},
    })
}
