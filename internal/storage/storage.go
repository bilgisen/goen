package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bilgisen/goen/internal/models"
)

// Storage defines the interface for news item persistence
type Storage interface {
	SaveNews(ctx context.Context, item *models.NewsItem) error
	GetNewsByID(ctx context.Context, id string) (*models.NewsItem, error)
	ListNews(ctx context.Context, page, pageSize int) ([]*models.NewsItem, error)
	DeleteNews(ctx context.Context, id string) error
	Close() error
}

// PurgeProcessed deletes all objects under the "processed/" prefix.
// This is a destructive operation meant for one-time cleanups.
func (r *R2Storage) PurgeProcessed(ctx context.Context) error {
    prefix := "processed/"

    var token *string
    for {
        // short timeout per list call
        listCtx, listCancel := context.WithTimeout(ctx, 8*time.Second)
        out, err := r.s3Client.ListObjectsV2(listCtx, &s3.ListObjectsV2Input{
            Bucket:            aws.String(r.bucket),
            Prefix:            aws.String(prefix),
            ContinuationToken: token,
            MaxKeys:           aws.Int32(1000),
        })
        listCancel()
        if err != nil {
            return fmt.Errorf("list objects failed: %w", err)
        }

        if len(out.Contents) == 0 {
            if !aws.ToBool(out.IsTruncated) {
                break
            }
            token = out.NextContinuationToken
            continue
        }

        // Build delete identifiers in batches (R2/S3 supports up to 1000 per request)
        ids := make([]types.ObjectIdentifier, 0, len(out.Contents))
        for _, obj := range out.Contents {
            ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
        }

        // short timeout per delete call
        delCtx, delCancel := context.WithTimeout(ctx, 10*time.Second)
        _, err = r.s3Client.DeleteObjects(delCtx, &s3.DeleteObjectsInput{
            Bucket: aws.String(r.bucket),
            Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
        })
        delCancel()
        if err != nil {
            return fmt.Errorf("delete objects failed: %w", err)
        }

        if !aws.ToBool(out.IsTruncated) {
            break
        }
        token = out.NextContinuationToken
    }

    return nil
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
func (s *FileStorage) SaveNews(ctx context.Context, item *models.NewsItem) error { // Added ctx parameter
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if context is done early
	if ctx.Err() != nil {
		return ctx.Err()
	}

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
func (s *FileStorage) GetNewsByID(ctx context.Context, id string) (*models.NewsItem, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // First, try to find the file directly by ID
    processedPath := s.basePath
    if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
        processedPath = filepath.Join(s.basePath, "processed")
    }

    // Look for the file in all date-based subdirectories
    yearDirs, err := os.ReadDir(processedPath)
    if err != nil {
        return nil, fmt.Errorf("error reading directory %s: %w", processedPath, err)
    }

    for _, yearDir := range yearDirs {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }

        if !yearDir.IsDir() {
            continue
        }

        monthDirs, err := os.ReadDir(filepath.Join(processedPath, yearDir.Name()))
        if err != nil {
            continue
        }

        for _, monthDir := range monthDirs {
            if ctx.Err() != nil {
                return nil, ctx.Err()
            }

            if !monthDir.IsDir() {
                continue
            }

            dayDirs, err := os.ReadDir(filepath.Join(processedPath, yearDir.Name(), monthDir.Name()))
            if err != nil {
                continue
            }

            for _, dayDir := range dayDirs {
                if ctx.Err() != nil {
                    return nil, ctx.Err()
                }

                if !dayDir.IsDir() {
                    continue
                }

                // Look for files in the day directory
                files, err := os.ReadDir(filepath.Join(processedPath, yearDir.Name(), monthDir.Name(), dayDir.Name()))
                if err != nil {
                    continue
                }

                for _, file := range files {
                    if !file.IsDir() && strings.Contains(file.Name(), id) && strings.HasSuffix(file.Name(), ".json") {
                        filePath := filepath.Join(processedPath, yearDir.Name(), monthDir.Name(), dayDir.Name(), file.Name())
                        data, err := os.ReadFile(filePath)
                        if err != nil {
                            return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
                        }

                        var item models.NewsItem
                        if err := json.Unmarshal(data, &item); err != nil {
                            return nil, fmt.Errorf("error unmarshaling news item: %w", err)
                        }

                        // Default featured to false when missing in older records
                        if item.Featured == nil {
                            def := false
                            item.Featured = &def
                        }

                        item.FilePath = filePath
                        return &item, nil
                    }
                }
            }
        }
    }

    return nil, fmt.Errorf("news item with ID %s not found", id)
}

// ListNews retrieves a paginated list of news items
func (s *FileStorage) ListNews(ctx context.Context, page, pageSize int) ([]*models.NewsItem, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var newsItems []*models.NewsItem
    processedPath := s.basePath
    if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
        processedPath = filepath.Join(s.basePath, "processed")
    }

    if page < 1 {
        page = 1
    }
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    // Check if context is done
    if ctx.Err() != nil {
        return nil, ctx.Err()
    }

    // Collect JSON files only from recent date folders (last 3 days)
    var files []string
    daysToScan := 3
    today := time.Now()
    for i := 0; i < daysToScan; i++ {
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }
        dayPath := filepath.Join(processedPath, today.AddDate(0, 0, -i).Format("2006/01/02"))
        entries, err := os.ReadDir(dayPath)
        if err != nil {
            continue
        }
        for _, e := range entries {
            if e.IsDir() {
                continue
            }
            name := e.Name()
            if strings.HasSuffix(name, ".json") {
                files = append(files, filepath.Join(dayPath, name))
            }
        }
        // Stop early if we already have enough candidates
        if len(files) >= page*pageSize*2 {
            break
        }
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
        if ctx.Err() != nil {
            return newsItems, ctx.Err()
        }
        data, err := os.ReadFile(file)
        if err != nil {
            return nil, fmt.Errorf("error reading file %s: %w", file, err)
        }

        var item models.NewsItem
        if err := json.Unmarshal(data, &item); err != nil {
            return nil, fmt.Errorf("error unmarshaling news item: %w", err)
        }

        // Default featured to false when missing in older records
        if item.Featured == nil {
            def := false
            item.Featured = &def
        }

        item.FilePath = file
        newsItems = append(newsItems, &item)
    }

    return newsItems, nil
}

// DeleteNews deletes a news item by its ID
func (s *FileStorage) DeleteNews(ctx context.Context, id string) error { // Added ctx parameter
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if context is done early
	if ctx.Err() != nil {
		return ctx.Err()
	}

	item, err := s.GetNewsByID(ctx, id) // Pass context here
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

// PurgeProcessedLocal deletes all JSON files under the local processed/ directory.
// Destructive; use cautiously.
func (s *FileStorage) PurgeProcessedLocal(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    processedPath := s.basePath
    if !strings.HasSuffix(strings.TrimRight(s.basePath, "/"), "processed") {
        processedPath = filepath.Join(s.basePath, "processed")
    }

    return filepath.Walk(processedPath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if ctx.Err() != nil {
            return ctx.Err()
        }
        if info.IsDir() {
            return nil
        }
        if strings.HasSuffix(info.Name(), ".json") {
            if remErr := os.Remove(path); remErr != nil {
                return fmt.Errorf("failed removing %s: %w", path, remErr)
            }
        }
        return nil
    })
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

func (r *R2Storage) SaveNews(ctx context.Context, item *models.NewsItem) error { // Added ctx parameter
	// Check if context is done early
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Marshal news item to JSON
	jsonData, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal news item: %w", err)
	}

	// Upload to R2 with dated path structure
	key := fmt.Sprintf("processed/%s/%s.json", time.Now().Format("2006/01/02"), item.ID)

	_, err = r.s3Client.PutObject(ctx, &s3.PutObjectInput{ // Use the provided context
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

func (r *R2Storage) GetNewsByID(ctx context.Context, id string) (*models.NewsItem, error) {
	// Use the provided context
	listResult, err := r.s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
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

	// Get the object from R2 using the context
	getResult, err := r.s3Client.GetObject(ctx, &s3.GetObjectInput{
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

	// Default featured to false when missing in older records
	if item.Featured == nil {
		def := false
		item.Featured = &def
	}

	// Update the item's file path for compatibility
	item.FilePath = foundKey

	return &item, nil
}

// ListNews retrieves a paginated list of news items
func (r *R2Storage) ListNews(ctx context.Context, page, pageSize int) ([]*models.NewsItem, error) {
    // Strategy: Only scan recent date prefixes (e.g., last 7 days) under processed/YYYY/MM/DD/
    // to avoid full-bucket scans that cause timeouts.

    if page < 1 {
        page = 1
    }
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    var collected []types.Object

    // Look back last 7 days to balance freshness with listing cost
    daysToScan := 7
    today := time.Now()

    for i := 0; i < daysToScan; i++ {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        day := today.AddDate(0, 0, -i)
        prefix := fmt.Sprintf("processed/%s/", day.Format("2006/01/02"))

        // Limit keys per day to avoid large scans
        var token *string
        for {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            default:
            }

            // Per-call timeout for listing to avoid long hangs
            listCtx, listCancel := context.WithTimeout(ctx, 6*time.Second)
            out, err := r.s3Client.ListObjectsV2(listCtx, &s3.ListObjectsV2Input{
                Bucket:            aws.String(r.bucket),
                Prefix:            aws.String(prefix),
                ContinuationToken: token,
                MaxKeys:           aws.Int32(int32(pageSize * 3)),
            })
            listCancel()
            if err != nil {
                return nil, fmt.Errorf("failed to list objects for prefix %s: %w", prefix, err)
            }

            for _, obj := range out.Contents {
                if strings.HasSuffix(aws.ToString(obj.Key), ".json") {
                    collected = append(collected, obj)
                }
            }

            if !aws.ToBool(out.IsTruncated) {
                break
            }
            token = out.NextContinuationToken

            // If we've already collected enough for pagination, we can break early
            if len(collected) >= page*pageSize { // enough for requested page
                break
            }
        }

        // Early exit if we already have enough for requested page
        if len(collected) >= page*pageSize {
            break
        }
    }

    // Sort newest first by LastModified
    sort.Slice(collected, func(i, j int) bool {
        return collected[i].LastModified.After(*collected[j].LastModified)
    })

    // Apply pagination window over collected objects
    start := (page - 1) * pageSize
    if start >= len(collected) {
        return []*models.NewsItem{}, nil
    }
    end := start + pageSize
    if end > len(collected) {
        end = len(collected)
    }

    var newsItems []*models.NewsItem
    var wg sync.WaitGroup
    var mu sync.Mutex
    errCh := make(chan error, end-start)
    // Limit concurrency to avoid throttling/timeouts
    sem := make(chan struct{}, 6)

    // Fetch and unmarshal each news item in parallel
    for _, obj := range collected[start:end] {
        select {
        case <-ctx.Done():
            return newsItems, ctx.Err()
        default:
        }

        wg.Add(1)
        go func(obj types.Object) {
            defer wg.Done()
            // acquire slot
            sem <- struct{}{}
            defer func() { <-sem }()

            // Per-call timeout for GetObject to avoid long hangs
            getCtx, getCancel := context.WithTimeout(ctx, 8*time.Second)
            getResult, err := r.s3Client.GetObject(getCtx, &s3.GetObjectInput{
                Bucket: aws.String(r.bucket),
                Key:    obj.Key,
            })
            getCancel()
            if err != nil {
                select {
                case errCh <- fmt.Errorf("failed to get object %s: %w", aws.ToString(obj.Key), err):
                case <-ctx.Done():
                }
                return
            }
            defer getResult.Body.Close()

            var item models.NewsItem
            if err := json.NewDecoder(getResult.Body).Decode(&item); err != nil {
                select {
                case errCh <- fmt.Errorf("failed to decode object %s: %w", aws.ToString(obj.Key), err):
                case <-ctx.Done():
                }
                return
            }

            // Default featured to false when missing in older records
            if item.Featured == nil {
                def := false
                item.Featured = &def
            }

            item.FilePath = aws.ToString(obj.Key)

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
    var errs []error
    for err := range errCh {
        if err != nil {
            errs = append(errs, err)
        }
    }

    // If we have at least one item, return partial results without failing the request
    if len(newsItems) > 0 {
        return newsItems, nil
    }

    if len(errs) > 0 {
        return newsItems, fmt.Errorf("encountered %d errors while fetching news items, first error: %w", len(errs), errs[0])
    }

    return newsItems, nil
}

// DeleteNews deletes a news item by its ID
func (r *R2Storage) DeleteNews(ctx context.Context, id string) error { // Added ctx parameter
	// Find the item first to get the key
	item, err := r.GetNewsByID(ctx, id) // Pass context here
	if err != nil {
		return err
	}

	// Delete the object from R2
	_, err = r.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
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

// EnsureProcessedExpiry configures lifecycle to expire processed/ objects after the given number of days.
func (r *R2Storage) EnsureProcessedExpiry(ctx context.Context, days int32) error {
    rule := types.LifecycleRule{
        ID:     aws.String("expire-processed"),
        Status: types.ExpirationStatusEnabled,
        Filter: &types.LifecycleRuleFilter{Prefix: aws.String("processed/")},
        Expiration: &types.LifecycleExpiration{
            Days: aws.Int32(days),
        },
    }

    _, err := r.s3Client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
        Bucket: aws.String(r.bucket),
        LifecycleConfiguration: &types.BucketLifecycleConfiguration{
            Rules: []types.LifecycleRule{rule},
        },
    })
    if err != nil {
        return fmt.Errorf("failed to set lifecycle configuration: %w", err)
    }
    return nil
}