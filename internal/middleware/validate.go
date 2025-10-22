package middleware

import (
    "net/http"

    "github.com/bilgisen/goen/internal/logger"
    "github.com/go-playground/validator/v10"
    "github.com/gofiber/fiber/v2"
)

// Validator is a struct that holds the validator instance
type Validator struct {
	validate *validator.Validate
}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	v := validator.New()
	// Add custom validators here if needed
	return &Validator{validate: v}
}

// Validate validates the request body against the provided struct
func (v *Validator) Validate(s interface{}) error {
	return v.validate.Struct(s)
}

// ValidateRequest is a middleware that validates the request body
func ValidateRequest(s interface{}) fiber.Handler {
	v := NewValidator()

	return func(c *fiber.Ctx) error {
		// Parse request body into the provided struct
		if err := c.BodyParser(s); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
				"msg":   err.Error(),
			})
		}

		// Validate the struct
		if err := v.Validate(s); err != nil {
			errors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				errors[err.Field()] = err.Tag()
			}

			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":  "Validation failed",
				"fields": errors,
			})
		}

		// Store the validated data in the context
		c.Locals("validated", s)

		return c.Next()
	}
}

// ValidateQueryParams validates query parameters
func ValidateQueryParams(s interface{}) fiber.Handler {
	v := NewValidator()

	return func(c *fiber.Ctx) error {
		// Parse query parameters into the provided struct
		if err := c.QueryParser(s); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid query parameters",
				"msg":   err.Error(),
			})
		}

		// Validate the struct
		if err := v.Validate(s); err != nil {
			errors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				errors[err.Field()] = err.Tag()
			}

			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":  "Invalid query parameters",
				"fields": errors,
			})
		}

		// Store the validated query params in the context
		c.Locals("queryParams", s)

		return c.Next()
	}
}

// ErrorHandler is a middleware that handles errors in a consistent way
func ErrorHandler(c *fiber.Ctx, err error) error {
    // Default status code
    code := fiber.StatusInternalServerError

    // Check if it's a fiber error
    if e, ok := err.(*fiber.Error); ok {
        code = e.Code
    }

    // Log the error
    logger.Get().Error().
        Err(err).
        Str("method", c.Method()).
        Str("path", c.Path()).
        Int("status", code).
        Msg("HTTP error")

    // Return JSON response
    return c.Status(code).JSON(fiber.Map{
        "error": http.StatusText(code),
    })
}
