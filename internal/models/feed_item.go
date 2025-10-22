package models

// FeedItem represents the Turkish source feed structure
type FeedItem struct {
	Guid      string `json:"guid"`
	TitleTR   string `json:"title"`
	ContentTR string `json:"content"`
	Image     string `json:"image"`
	Url       string `json:"url"`
	Category  string `json:"category"`
}
