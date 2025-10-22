package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilgisen/goen/internal/models"
)

type Storage struct {
	basePath string
	mu      sync.RWMutex
}

func NewStorage(basePath string) (*Storage, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create processed directory if it doesn't exist
	processedPath := filepath.Join(basePath, "processed")
	if err := os.MkdirAll(processedPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create processed directory: %w", err)
	}

	return &Storage{
		basePath: basePath,
	}, nil
}

// SaveNews saves a news item to disk
func (s *Storage) SaveNews(ctx context.Context, item *models.NewsItem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.mu.Lock()
		defer s.mu.Unlock()

		// Create dated directory (YYYY/MM/DD)
		datePath := filepath.Join(s.basePath, "processed", time.Now().Format("2006/01/02"))
		if err := os.MkdirAll(datePath, 0755); err != nil {
			return fmt.Errorf("failed to create date directory: %w", err)
		}

		// Create filename with timestamp and ID
		filename := fmt.Sprintf("%d_%s.json", time.Now().Unix(), item.ID)
		filePath := filepath.Join(datePath, filename)

		// Marshal to JSON
		data, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal news item: %w", err)
		}

		// Write to file
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write news file: %w", err)
		}

		// Update the item's file path
		item.FilePath = filePath

		return nil
	}
}

// GetNewsByID retrieves a news item by its ID
func (s *Storage) GetNewsByID(ctx context.Context, id string) (*models.NewsItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		s.mu.RLock()
		defer s.mu.RUnlock()

		var foundItem *models.NewsItem
		err := filepath.WalkDir(filepath.Join(s.basePath, "processed"), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
				return nil
			}

			// Check if this is the file we're looking for
			if strings.Contains(d.Name(), id) {
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read file %s: %w", path, err)
				}

				var item models.NewsItem
				if err := json.Unmarshal(data, &item); err != nil {
					return fmt.Errorf("failed to unmarshal news item: %w", err)
				}

				foundItem = &item
				foundItem.FilePath = path
				return filepath.SkipDir
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("error walking the path: %w", err)
		}

		if foundItem == nil {
			return nil, fmt.Errorf("news item with ID %s not found", id)
		}

		return foundItem, nil
	}
}

// ListNews retrieves a paginated list of news items
func (s *Storage) ListNews(ctx context.Context, page, pageSize int) ([]*models.NewsItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		s.mu.RLock()
		defer s.mu.RUnlock()

		var newsItems []*models.NewsItem
		processedPath := filepath.Join(s.basePath, "processed")

		// Get all JSON files
		var files []string
		err := filepath.Walk(processedPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
				files = append(files, path)
			}
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("error walking the path: %w", err)
		}

		// Sort by modification time (newest first)
		sort.Slice(files, func(i, j int) bool {
			info1, _ := os.Stat(files[i])
			info2, _ := os.Stat(files[j])
			return info1.ModTime().After(info2.ModTime())
		})

		// Apply pagination
		start := (page - 1) * pageSize
		if start >= len(files) {
			return []*models.NewsItem{}, nil
		}

		end := start + pageSize
		if end > len(files) {
			end = len(files)
		}

		// Read and unmarshal the files
		for _, file := range files[start:end] {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, fmt.Errorf("error reading file %s: %w", file, err)
			}

			var item models.NewsItem
			if err := json.Unmarshal(data, &item); err != nil {
				return nil, fmt.Errorf("error unmarshaling news item: %w", err)
			}

			item.FilePath = file
			newsItems = append(newsItems, &item)
		}

		return newsItems, nil
	}
}

// DeleteNews deletes a news item by its ID
func (s *Storage) DeleteNews(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.mu.Lock()
		defer s.mu.Unlock()

		item, err := s.GetNewsByID(ctx, id)
		if err != nil {
			return err
		}

		if err := os.Remove(item.FilePath); err != nil {
			return fmt.Errorf("failed to delete news file: %w", err)
		}

		return nil
	}
}
