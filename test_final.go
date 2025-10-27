package main
import (
    "fmt"
    "github.com/bilgisen/goen/internal/config"
)
func main() {
    cfg := config.Load()
    fmt.Printf("✅ Interface implementation completed!
")
    fmt.Printf("Redis URL: %s
", cfg.RedisURL)
    fmt.Printf("AI Model: %s
", cfg.AIModel)
    fmt.Printf("R2 Endpoint: %s
", cfg.R2Endpoint)
    fmt.Printf("R2 Bucket: %s
", cfg.R2Bucket)
}
