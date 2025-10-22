package ai

import (
	"fmt"
	"strings"
)

// PromptTemplates contains various prompt templates for different types of content generation
var PromptTemplates = struct {
	NewsArticle string
}{
	NewsArticle: `You are an expert English journalist and SEO specialist. 
Transform the following Turkish news article into a professional English version with these requirements:

1. SEO Title: Catchy, under 60 characters
2. SEO Description: 1-2 sentences, under 160 characters
3. TLDR: 3 key points as bullet points
4. Main Content: Well-structured markdown with paragraphs and proper formatting
5. Category: Keep the original category
6. Tags: 5-7 relevant keywords
7. Image Metadata: Title and description for accessibility

Format your response as a valid JSON object with these fields:
- seo_title (string)
- seo_description (string)
- tldr (array of strings)
- content_md (markdown formatted string)
- category (string)
- tags (array of strings)
- image_title (string)
- image_description (string)

Turkish Article:
Title: %s

Content: %s

Category: %s`,
}

// BuildNewsPrompt creates a prompt for news article translation
func BuildNewsPrompt(title, content, category string) string {
	title = escapeForPrompt(title)
	content = escapeForPrompt(content)
	category = escapeForPrompt(category)

	return fmt.Sprintf(PromptTemplates.NewsArticle, title, content, category)
}

// escapeForPrompt escapes special characters for use in prompts
func escapeForPrompt(s string) string {
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `
`, " ")
	s = strings.ReplaceAll(s, `	`, " ")
	return strings.TrimSpace(s)
}

// ResponseTemplate defines the expected JSON structure of the AI's response
type ResponseTemplate struct {
	SeoTitle    string   `json:"seo_title"`
	SeoDesc     string   `json:"seo_description"`
	TLDR        []string `json:"tldr"`
	ContentMD   string   `json:"content_md"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	ImageTitle  string   `json:"image_title"`
	ImageDesc   string   `json:"image_description"`
}
