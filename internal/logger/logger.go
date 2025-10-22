package logger

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "sync"
    "time"

    "github.com/rs/zerolog"
)

// These constants are the string representation of the log levels
const (
    // DebugLevel defines debug log level
    DebugLevel = "debug"
    // InfoLevel defines info log level
    InfoLevel = "info"
    // WarnLevel defines warn log level
    WarnLevel = "warn"
    // ErrorLevel defines error log level
    ErrorLevel = "error"
    // FatalLevel defines fatal log level
    FatalLevel = "fatal"
    // PanicLevel defines panic log level
    PanicLevel = "panic"
    // NoLevel defines no log level
    NoLevel = ""
    // Disabled disables the logger
    Disabled = "disabled"
)

var (
    once   sync.Once
    logger zerolog.Logger
)

// Config holds the configuration for the logger
type Config struct {
    Level  string
    Output string // "stdout", "stderr", or file path
    Pretty bool   // Enable pretty logging for development
}

// Init initializes the global logger
func Init(cfg Config) error {
    var err error
    once.Do(func() {
        // Set log level
        level, parseErr := zerolog.ParseLevel(strings.ToLower(cfg.Level))
        if parseErr != nil {
            level = zerolog.InfoLevel
        }
        zerolog.SetGlobalLevel(level)

        // Set time format
        zerolog.TimeFieldFormat = time.RFC3339Nano

        // Create writer based on config
        var output io.Writer
        switch cfg.Output {
        case "stdout":
            output = os.Stdout
        case "stderr":
            output = os.Stderr
        default:
            // Try to create the directory if it doesn't exist
            dir := filepath.Dir(cfg.Output)
            if dir != "." && dir != string(filepath.Separator) {
                if err := os.MkdirAll(dir, 0755); err != nil {
                    fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
                    output = os.Stdout
                    break
                }
            }
            
            file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
            if err != nil {
                fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
                output = os.Stdout
                break
            }
            output = file
        }

        // Create logger
        if cfg.Pretty {
            logger = zerolog.New(zerolog.ConsoleWriter{
                Out:        output,
                TimeFormat: "2006-01-02 15:04:05",
            })
        } else {
            logger = zerolog.New(output)
        }

        // Add timestamp and caller info
        logger = logger.With().
            Timestamp().
            Caller().
            Logger()

        // Set default logger for any package that uses the global logger
        zerolog.DefaultContextLogger = &logger
    })
    return err
}

// Get returns the logger instance
func Get() *zerolog.Logger {
    return &logger
}

// WithContext adds context to the logger
func WithContext(ctx context.Context) *zerolog.Logger {
    l := logger.With().Logger()
    return &l
}

// Helper functions for different log levels
func Debug() *zerolog.Event {
    return logger.Debug().Caller(1)
}

func Info() *zerolog.Event {
    return logger.Info().Caller(1)
}

func Warn() *zerolog.Event {
    return logger.Warn().Caller(1)
}

func Error() *zerolog.Event {
    return logger.Error().Caller(1)
}

func Fatal() *zerolog.Event {
    return logger.Fatal().Caller(1)
}

// WithError adds an error to the log context
func WithError(err error) *zerolog.Event {
    return logger.Error().Err(err)
}

// WithField adds a field to the log context
func WithField(key string, value interface{}) *zerolog.Event {
    l := Get().With().Interface(key, value).Logger()
    return l.Info().Str("caller", getCaller())
}

// WithFields adds multiple fields to the log context
func WithFields(fields map[string]interface{}) *zerolog.Event {
    ctx := Get().With()
    for k, v := range fields {
        ctx = ctx.Interface(k, v)
    }
    l := ctx.Logger()
    return l.Info().Str("caller", getCaller())
}

// getCaller returns the caller's file and line number
func getCaller() string {
    _, file, line, ok := runtime.Caller(3) // 3 levels up the stack
    if !ok {
        return "unknown:0"
    }
    return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
