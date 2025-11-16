package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	// Server configuration
	Port            string        `json:"port"`
	Env             string        `json:"env"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	HTTPTimeout     time.Duration `json:"http_timeout"`

	// Redis configuration
	RedisURL       string `json:"redis_url"` // Redis connection URL for rate limiting and caching
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
	AIApiKey     string `json:"ai_api_key"`
	AIModel      string `json:"ai_model"`
	AITimeout    int    `json:"ai_timeout"`
	AIMaxTokens  int    `json:"ai_max_tokens"`
	AIRateLimit  int    `json:"ai_rate_limit"`  // Max requests per minute
	AITPMLimit   int    `json:"ai_tpm_limit"`   // Max tokens per minute

	// Storage
	StoragePath    string   `json:"storage_path"`
	FeedSourcePath string   `json:"feed_source_path"`
	FeedURLs       []string `json:"feed_urls"` // Default feed URLs to process
	ProcessedPath  string   `json:"processed_path"`
	RetentionDays  int      `json:"retention_days"`
	MaxFileSize    int64    `json:"max_file_size"`

	// Logging
	LogLevel string `json:"log_level"`
	LogFile  string `json:"log_file"`

	// Security
	AdminAPIKey    string `json:"admin_api_key"`
	ExternalApiKey string `json:"external_api_key"`
}

// Load loads configuration from config.yaml and environment variables
// Environment variables take priority over config.yaml
func Load() *Config {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: Error loading .env file: %v", err)
	}
	
	// Load environment variables for Redis
	// This allows different Redis URLs for different environments
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0" // Default for development
	}

	// Set up Viper to read config.yaml
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Try to read config.yaml
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Could not read config.yaml: %v", err)
	}

	// Set environment variable prefix (optional)
	viper.SetEnvPrefix("")
	viper.AutomaticEnv() // Read environment variables

	cfg := &Config{
		// Server configuration
		Port:            viper.GetString("server.port"),
		Env:             viper.GetString("server.environment"),
		ShutdownTimeout: viper.GetDuration("server.shutdown_timeout"),
		HTTPTimeout:     viper.GetDuration("server.http_timeout"),

		// Redis configuration - using environment variable if set
		RedisURL:       redisURL, // From environment variable or default
		RedisPrefix:    viper.GetString("redis.prefix"),
		CacheTTL:       viper.GetDuration("redis.cache_ttl"),
		MaxConcurrency: viper.GetInt("redis.max_concurrency"),

		// AI Configuration
		AIApiKey:    viper.GetString("AI_API_KEY"),
		AIModel:     viper.GetString("AI_MODEL"),
		AITimeout:   viper.GetInt("AI_TIMEOUT"),
		AIMaxTokens: viper.GetInt("AI_MAX_TOKENS"),
		AIRateLimit: viper.GetInt("AI_RATE_LIMIT"),
		AITPMLimit:  viper.GetInt("AI_TPM_LIMIT"),

		// Storage
		StoragePath:    viper.GetString("STORAGE_PATH"),
		FeedSourcePath: viper.GetString("FEED_SOURCE_PATH"),
		FeedURLs:       viper.GetStringSlice("FEED_URLS"),
		ProcessedPath:  viper.GetString("PROCESSED_PATH"),
		RetentionDays:  viper.GetInt("RETENTION_DAYS"),
		MaxFileSize:    viper.GetInt64("MAX_FILE_SIZE"),

		// CloudFlare R2 Configuration
		R2Endpoint:  viper.GetString("R2_ENDPOINT"),
		R2AccessKey: viper.GetString("R2_ACCESS_KEY"),
		R2SecretKey: viper.GetString("R2_SECRET_ACCESS_KEY"),
		R2Bucket:    viper.GetString("R2_BUCKET"),
		R2AccountID: viper.GetString("CLOUDFLARE_ACCOUNT_ID"),

		// Logging
		LogLevel: viper.GetString("LOG_LEVEL"),
		LogFile:  viper.GetString("LOG_FILE"),

		// Security
		AdminAPIKey:    viper.GetString("ADMIN_API_KEY"),
		ExternalApiKey: viper.GetString("EXTERNAL_API_KEY"),
	}

	// Apply defaults if values are empty
	applyDefaults(cfg)

	return cfg
}

// applyDefaults sets default values if configuration values are empty
func applyDefaults(cfg *Config) {
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.Env == "" {
		cfg.Env = "development"
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}
	if cfg.HTTPTimeout == 0 {
		// Increased to 75s to accommodate rate limiter's full wait time (60s) + buffer
		cfg.HTTPTimeout = 75 * time.Second
	}
	if cfg.RedisURL == "" {
		cfg.RedisURL = "redis://localhost:6379/0"
	}
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = "ai-news:"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 720 * time.Hour // 30 days
	}
	if cfg.MaxConcurrency == 0 {
		cfg.MaxConcurrency = 5
	}
	if cfg.AIModel == "" {
		cfg.AIModel = "gemini-pro"
	}
	if cfg.AITimeout == 0 {
		cfg.AITimeout = 60
	}
	if cfg.AIMaxTokens == 0 {
		cfg.AIMaxTokens = 2000
	}
	if cfg.AIRateLimit == 0 {
		cfg.AIRateLimit = 15 // Default to 15 requests per minute
	}
	if cfg.AITPMLimit == 0 {
		cfg.AITPMLimit = 250000 // Default to 250K tokens per minute
	}
	if cfg.RedisURL == "" {
		cfg.RedisURL = os.Getenv("REDIS_URL") // Fallback to env var if not set in config
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = "./data"
	}
	if cfg.FeedSourcePath == "" {
		cfg.FeedSourcePath = "./data/feeds/"
	}
	if len(cfg.FeedURLs) == 0 {
		// Set some default news feed URLs if none provided
		cfg.FeedURLs = []string{
			"https://www.ekonomim.com/rss",
			"https://www.ntv.com.tr/spor.rss",
			"https://www.ntv.com.tr/dunya.rss",
		}
	}
	if cfg.ProcessedPath == "" {
		cfg.ProcessedPath = "./data/processed/"
	}
	if cfg.R2Bucket == "" {
		cfg.R2Bucket = "newsapi"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Add validation logic here
	return nil
}
