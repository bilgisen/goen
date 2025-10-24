package models

import "time"

// NewsItem represents the generated English content
type NewsItem struct {
	ID           string    `json:"id"`
	SourceGuid   string    `json:"source_guid"`
	SeoTitle     string    `json:"seo_title"`
	SeoDesc      string    `json:"seo_description"`
	TLDR         []string  `json:"tldr"`
	ContentMD    string    `json:"content_md"`
	Category     string    `json:"category"`
	Tags         []string  `json:"tags"`
	Image        string    `json:"image"`
	ImageTitle   string    `json:"image_title"`
	ImageDesc    string    `json:"image_desc"`
	OriginalUrl  string    `json:"original_url"`
	FilePath     string    `json:"file_path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	PublishedAt  time.Time `json:"published_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}
