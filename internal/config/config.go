package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
// Config holds all configuration for the application
type Config struct {
	// Server configuration
	Port            string        `json:"port"`
	Env             string        `json:"env"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	HTTPTimeout     time.Duration `json:"http_timeout"`

	// Redis configuration
	RedisURL       string `json:"redis_url"`
	RedisPrefix    string `json:"redis_prefix"`
	CacheTTL       time.Duration `json:"cache_ttl"`
	MaxConcurrency int    `json:"max_concurrency"`

	// CloudFlare R2 Configuration
	R2Endpoint      string `json:"r2_endpoint"`
	R2AccessKey     string `json:"r2_access_key"`
	R2SecretKey     string `json:"r2_secret_key"`
	R2Bucket        string `json:"r2_bucket"`
	R2AccountID     string `json:"r2_account_id"`

	// AI Configuration
	AIApiKey    string `json:"ai_api_key"`
	AIModel     string `json:"ai_model"`
	AITimeout   int    `json:"ai_timeout"`
	AIMaxTokens int    `json:"ai_max_tokens"`

	// Storage
	StoragePath    string `json:"storage_path"`
	FeedSourcePath string `json:"feed_source_path"`
	ProcessedPath  string `json:"processed_path"`
	RetentionDays  int    `json:"retention_days"`
	MaxFileSize    int64  `json:"max_file_size"`

	// Logging
	LogLevel string `json:"log_level"`
	LogFile  string `json:"log_file"`

	// Security
	AdminAPIKey string `json:"admin_api_key"`
}

// Load loads configuration from environment variables and validates it
func Load() *Config {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	cfg := &Config{
		// Server configuration
		Port:            getEnv("PORT", "8080"),
		Env:             getEnv("APP_ENV", "development"),
		ShutdownTimeout: getEnvAsDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HTTPTimeout:     getEnvAsDuration("HTTP_TIMEOUT", 30*time.Second),

		// Redis configuration
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379/0"),
		RedisPrefix:    getEnv("REDIS_PREFIX", "ai-news:"),
		CacheTTL:       getEnvAsDuration("CACHE_TTL", 720*time.Hour), // 30 days
		MaxConcurrency: getEnvAsInt("MAX_CONCURRENCY", 5),

		// AI Configuration
		AIApiKey:    getEnv("AI_API_KEY", ""),
		AIModel:     getEnv("AI_MODEL", "gemini-pro"),
		AITimeout:   getEnvAsInt("AI_TIMEOUT", 60),
		AIMaxTokens: getEnvAsInt("AI_MAX_TOKENS", 2000),

		// Storage
		StoragePath:    getEnv("STORAGE_PATH", "./data"),
		FeedSourcePath: getEnv("FEED_SOURCE_PATH", "./data/feeds/"),
		ProcessedPath:  getEnv("PROCESSED_PATH", "./data/processed/"),
		MaxFileSize:    getEnvAsInt64("MAX_FILE_SIZE", 10<<20), // 10MB
		RetentionDays:  getEnvAsInt("RETENTION_DAYS", 30),

		// CloudFlare R2 Configuration
		R2Endpoint:  getEnv("R2_ENDPOINT", ""),
		R2AccessKey: getEnv("R2_ACCESS_KEY", ""),
		R2SecretKey: getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:    getEnv("R2_BUCKET", "newsapi"),
		R2AccountID: getEnv("CLOUDFLARE_ACCOUNT_ID", ""),

		// Logging
		LogLevel: getEnv("LOG_LEVEL", "info"),
		LogFile:  getEnv("LOG_FILE", ""),

		// Security
		AdminAPIKey: getEnv("ADMIN_API_KEY", ""),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	return cfg
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Add validation logic here
	return nil
}

// Helper functions for environment variable handling
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(name string, defaultVal int) int {
	valueStr := getEnv(name, "")
	if valueStr == "" {
		return defaultVal
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Invalid %s value: %v, using default: %d", name, err, defaultVal)
		return defaultVal
	}
	return value
}

func getEnvAsInt64(name string, defaultVal int64) int64 {
	valueStr := getEnv(name, "")
	if valueStr == "" {
		return defaultVal
	}
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		log.Printf("Invalid %s value: %v, using default: %d", name, err, defaultVal)
		return defaultVal
	}
	return value
}

func getEnvAsDuration(name string, defaultVal time.Duration) time.Duration {
	valueStr := getEnv(name, "")
	if valueStr == "" {
		return defaultVal
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		log.Printf("Invalid %s value: %v, using default: %v", name, err, defaultVal)
		return defaultVal
	}
	return value
}
