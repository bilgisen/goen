package middleware

import (
    "errors"
    "strings"

    "github.com/bilgisen/goen/internal/logger"
    "github.com/gofiber/fiber/v2"
)

// AuthConfig defines the config for the auth middleware
type AuthConfig struct {
	// Skip defines a function to skip middleware.
	// Optional. Default: nil
	Next func(c *fiber.Ctx) bool

	// Validator is a function to validate the API key.
	// Required.
	Validator func(key string) (bool, error)

	// ErrorHandler defines a function which is executed for an invalid API key.
	// Optional. Default: 401 Invalid or missing API Key
	ErrorHandler fiber.ErrorHandler

	// ContextKey is the key used to store the API key in the context.
	// Optional. Default: "apiKey"
	ContextKey string

	// Header is the header key where to get the API key from.
	// Optional. Default: "X-API-Key"
	Header string
}

// ConfigDefault is the default config
var ConfigDefault = AuthConfig{
	Next: nil,
	ErrorHandler: func(c *fiber.Ctx, err error) error {
		logger.Get().Error().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("ip", c.IP()).
			Err(err).
			Msg("Authentication failed")

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or missing API Key",
		})
	},
	ContextKey: "apiKey",
	Header:     "X-API-Key",
}

// New creates a new middleware handler
func NewAuth(config ...AuthConfig) fiber.Handler {
	// Set default config
	cfg := ConfigDefault

	// Override config if provided
	if len(config) > 0 {
		cfg = config[0]

		// Set default values
		if cfg.Next == nil {
			cfg.Next = ConfigDefault.Next
		}
		if cfg.ErrorHandler == nil {
			cfg.ErrorHandler = ConfigDefault.ErrorHandler
		}
		if cfg.ContextKey == "" {
			cfg.ContextKey = ConfigDefault.ContextKey
		}
		if cfg.Header == "" {
			cfg.Header = ConfigDefault.Header
		}
	}

	// Return new handler
	return func(c *fiber.Ctx) error {
		// Don't execute middleware if Next returns true
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		// Get API key from header
		authHeader := c.Get(cfg.Header)
		if authHeader == "" {
			return cfg.ErrorHandler(c, errors.New("missing API key"))
		}

		// For "Bearer " prefixed tokens
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the API key
		valid, err := cfg.Validator(token)
		if err != nil {
			return cfg.ErrorHandler(c, err)
		}

		if !valid {
			return cfg.ErrorHandler(c, errors.New("invalid API key"))
		}

		// Store the API key in the context
		c.Locals(cfg.ContextKey, token)

		// Continue stack
		return c.Next()
	}
}

// AdminOnly is a middleware that checks if the request is from an admin
func AdminOnly(adminKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get API key from header
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			logger.Get().Warn().
				Str("method", c.Method()).
				Str("path", c.Path()).
				Str("ip", c.IP()).
				Msg("Admin access attempt without API key")

			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "API key is required",
			})
		}

		// Check if the API key matches the admin key
		if apiKey != adminKey {
			logger.Get().Warn().
				Str("method", c.Method()).
				Str("path", c.Path()).
				Str("ip", c.IP()).
				Str("api_key", apiKey).
				Msg("Unauthorized admin access attempt")

			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Admin access required",
			})
		}

		return c.Next()
	}
}
