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
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Storage defines the interface for news item persistence
type Storage interface {
	SaveNews(item *models.NewsItem) error
	GetNewsByID(id string) (*models.NewsItem, error)
	ListNews(page, pageSize int) ([]*models.NewsItem, error)
	DeleteNews(id string) error
	Close() error
}

// FileStorage implements Storage interface using local filesystem
type FileStorage struct {
	basePath string
	mu      sync.RWMutex
}

// NewFileStorage creates a new file-based storage instance
func NewFileStorage(basePath string) (*FileStorage, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create processed directory if it doesn't exist
	processedPath := filepath.Join(basePath, "processed")
	if err := os.MkdirAll(processedPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create processed directory: %w", err)
	}

	return &FileStorage{
		basePath: basePath,
	}, nil
}

// NewStorage creates a new file-based storage (backward compatibility)
func NewStorage(basePath string) (*FileStorage, error) {
	return NewFileStorage(basePath)
}

// SaveNews saves a news item to disk
func (s *FileStorage) SaveNews(item *models.NewsItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create dated directory (YYYY/MM/DD)
	datePath := s.basePath
	if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
		datePath = filepath.Join(s.basePath, "processed")
	}
	datePath = filepath.Join(datePath, time.Now().Format("2006/01/02"))
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

// GetNewsByID retrieves a news item by its ID
func (s *FileStorage) GetNewsByID(id string) (*models.NewsItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var foundItem *models.NewsItem
	searchPath := s.basePath
	if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
		searchPath = filepath.Join(s.basePath, "processed")
	}
	err := filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
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

// ListNews retrieves a paginated list of news items
func (s *FileStorage) ListNews(page, pageSize int) ([]*models.NewsItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var newsItems []*models.NewsItem
	processedPath := s.basePath
	// If basePath already ends with "processed" or "processed/", use it directly
	// Otherwise, append "processed" to it
	if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
		processedPath = filepath.Join(s.basePath, "processed")
	}

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

// DeleteNews deletes a news item by its ID
func (s *FileStorage) DeleteNews(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.GetNewsByID(id)
	if err != nil {
		return err
	}

	if err := os.Remove(item.FilePath); err != nil {
		return fmt.Errorf("failed to delete news file: %w", err)
	}

	return nil
}

// Close closes the file storage (no-op for filesystem storage)
func (s *FileStorage) Close() error {
	// Filesystem operations don't require explicit cleanup
	return nil
}
type R2Storage struct {
	s3Client *s3.Client
	bucket   string
}

// NewR2Storage creates a new R2-based storage instance
func NewR2Storage(endpoint, accessKey, secretKey, bucket, accountID string) (*R2Storage, error) {
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("R2 configuration is incomplete")
	}

	customCfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey, secretKey, "")),
		awsConfig.WithRegion("auto"),
		awsConfig.WithEndpointResolver(aws.EndpointResolverFunc(
			func(service, region string) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
			})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load R2 config: %w", err)
	}

	// Create S3 client for R2
	s3Client := s3.NewFromConfig(customCfg)

	return &R2Storage{
		s3Client: s3Client,
		bucket:   bucket,
	}, nil
}
func (r *R2Storage) SaveNews(item *models.NewsItem) error {
	// Marshal news item to JSON
	jsonData, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal news item: %w", err)
	}

	// Upload to R2 with dated path structure
	key := fmt.Sprintf("processed/%s/%s.json", time.Now().Format("2006/01/02"), item.ID)

	_, err = r.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(string(jsonData)),
		ContentType: aws.String("application/json"),
	})

	if err != nil {
		return fmt.Errorf("failed to upload to R2: %w", err)
	}

	// Update the item's file path for compatibility
	item.FilePath = key

	return nil
}
func (r *R2Storage) GetNewsByID(id string) (*models.NewsItem, error) {
	// Try to find the item in R2 by listing objects with the ID
	listResult, err := r.s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: aws.String(r.bucket),
		Prefix: aws.String("processed/"),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list objects in R2: %w", err)
	}

	// Search for the item with the matching ID
	var foundKey string
	for _, obj := range listResult.Contents {
		if strings.Contains(*obj.Key, id+".json") {
			foundKey = *obj.Key
			break
		}
	}

	if foundKey == "" {
		return nil, fmt.Errorf("news item with ID %s not found", id)
	}

	// Get the object from R2
	getResult, err := r.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(foundKey),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get object from R2: %w", err)
	}
	defer getResult.Body.Close()

	// Unmarshal the JSON data
	var item models.NewsItem
	if err := json.NewDecoder(getResult.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal news item: %w", err)
	}

	// Update the item's file path for compatibility
	item.FilePath = foundKey

	return &item, nil
}
func (r *R2Storage) ListNews(page, pageSize int) ([]*models.NewsItem, error) {
    var allObjects []types.Object
    var continuationToken *string

    // First, list all objects with pagination
    for {
        listResult, err := r.s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
            Bucket:            aws.String(r.bucket),
            Prefix:            aws.String("processed/"),
            ContinuationToken: continuationToken,
        })

        if err != nil {
            return nil, fmt.Errorf("failed to list objects in R2: %w", err)
        }

        // Filter only JSON files
        for _, obj := range listResult.Contents {
            if strings.HasSuffix(*obj.Key, ".json") {
                allObjects = append(allObjects, obj)
            }
        }

        // Check if there are more objects to fetch
        if !aws.ToBool(listResult.IsTruncated) {
            break
        }
        continuationToken = listResult.NextContinuationToken
    }

    // Sort by last modified time (newest first)
    sort.Slice(allObjects, func(i, j int) bool {
        return allObjects[i].LastModified.After(*allObjects[j].LastModified)
    })

    // Apply pagination
    start := (page - 1) * pageSize
    if start >= len(allObjects) {
        return []*models.NewsItem{}, nil
    }

    end := start + pageSize
    if end > len(allObjects) {
        end = len(allObjects)
    }

    var newsItems []*models.NewsItem
    var wg sync.WaitGroup
    var mu sync.Mutex
    errCh := make(chan error, end-start)

    // Fetch and unmarshal each news item in parallel
    for _, obj := range allObjects[start:end] {
        wg.Add(1)
        go func(obj types.Object) {
            defer wg.Done()

            getResult, err := r.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
                Bucket: aws.String(r.bucket),
                Key:    obj.Key,
            })
            if err != nil {
                errCh <- fmt.Errorf("failed to get object %s: %w", *obj.Key, err)
                return
            }
            defer getResult.Body.Close()

            var item models.NewsItem
            if err := json.NewDecoder(getResult.Body).Decode(&item); err != nil {
                errCh <- fmt.Errorf("failed to decode object %s: %w", *obj.Key, err)
                return
            }

            item.FilePath = *obj.Key
            
            mu.Lock()
            newsItems = append(newsItems, &item)
            mu.Unlock()
        }(obj)
    }

    // Wait for all goroutines to complete
    go func() {
        wg.Wait()
        close(errCh)
    }()

    // Collect any errors
    var errors []error
    for err := range errCh {
        errors = append(errors, err)
    }

    if len(errors) > 0 {
        return newsItems, fmt.Errorf("encountered %d errors while fetching news items, first error: %w", len(errors), errors[0])
    }

    return newsItems, nil
}
func (r *R2Storage) DeleteNews(id string) error {
	// Find the item first to get the key
	item, err := r.GetNewsByID(id)
	if err != nil {
		return err
	}

	// Delete the object from R2
	_, err = r.s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(item.FilePath),
	})

	if err != nil {
		return fmt.Errorf("failed to delete object from R2: %w", err)
	}

	return nil
}

// Close closes the R2 storage connection
func (r *R2Storage) Close() error {
	// No explicit cleanup needed for S3 client
	return nil
}
