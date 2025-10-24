package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewsItemImageField(t *testing.T) {
	// Test that the Image field is properly serialized in JSON
	now := time.Now()
	newsItem := NewsItem{
		ID:          "test-id",
		SourceGuid:  "test-guid",
		SeoTitle:    "Test Title",
		SeoDesc:     "Test Description",
		TLDR:        []string{"Point 1", "Point 2", "Point 3"},
		ContentMD:   "# Test Content\n\nThis is test content.",
		Category:    "general",
		Tags:        []string{"test", "news"},
		Image:       "https://example.com/image.jpg",
		ImageTitle:  "Test Image Title",
		ImageDesc:   "Test Image Description",
		OriginalUrl: "https://example.com/news",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Marshal to JSON
	data, err := json.Marshal(newsItem)
	if err != nil {
		t.Fatalf("Failed to marshal NewsItem: %v", err)
	}

	// Check that Image field is present in JSON
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify Image field exists and has correct value
	if result["image"] != "https://example.com/image.jpg" {
		t.Errorf("Expected image field to be 'https://example.com/image.jpg', got %v", result["image"])
	}

	// Verify ImageTitle and ImageDesc fields are also present
	if result["image_title"] != "Test Image Title" {
		t.Errorf("Expected image_title field to be 'Test Image Title', got %v", result["image_title"])
	}

	if result["image_desc"] != "Test Image Description" {
		t.Errorf("Expected image_desc field to be 'Test Image Description', got %v", result["image_desc"])
	}
}
