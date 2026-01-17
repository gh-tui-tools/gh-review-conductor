package cmd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/chmouel/gh-prreview/pkg/github"
	"github.com/chmouel/gh-prreview/pkg/ui"
)

func TestBrowseItemRenderer_IsSkippable(t *testing.T) {
	renderer := &browseItemRenderer{
		repo:           "owner/repo",
		prNumber:       123,
		collapsedFiles: make(map[string]bool),
	}

	tests := []struct {
		name     string
		item     BrowseItem
		expected bool
	}{
		{
			name: "file header is not skippable",
			item: BrowseItem{
				Type: "file",
				Path: "src/main.go",
			},
			expected: false,
		},
		{
			name: "comment is not skippable",
			item: BrowseItem{
				Type: "comment",
				Path: "src/main.go",
				Comment: &github.ReviewComment{
					ID:     123,
					Author: "reviewer",
					Body:   "Consider refactoring this",
				},
			},
			expected: false,
		},
		{
			name: "preview is not skippable (prevents strikethrough styling)",
			item: BrowseItem{
				Type:      "comment_preview",
				Path:      "src/main.go",
				IsPreview: true,
				Comment: &github.ReviewComment{
					ID:     123,
					Author: "reviewer",
					Body:   "Consider refactoring this",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.IsSkippable(tt.item)
			if result != tt.expected {
				t.Errorf("IsSkippable(%v) = %v, want %v", tt.item.Type, result, tt.expected)
			}
		})
	}
}

func TestBrowseItemRenderer_Title_PreviewUsesGrayColor(t *testing.T) {
	renderer := &browseItemRenderer{
		repo:           "owner/repo",
		prNumber:       123,
		collapsedFiles: make(map[string]bool),
	}

	item := BrowseItem{
		Type:      "comment_preview",
		Path:      "src/main.go",
		IsPreview: true,
		Comment: &github.ReviewComment{
			ID:     123,
			Author: "reviewer",
			Body:   "Consider refactoring this function for better readability",
		},
	}

	title := renderer.Title(item)

	// Title should contain the preview text
	if !strings.Contains(title, "Consider refactoring") {
		t.Errorf("Title should contain preview text, got: %q", title)
	}

	// Title should use gray color (ANSI code 90) when colors are enabled
	if ui.ColorsEnabled() {
		if !strings.Contains(title, ui.ColorGray) {
			t.Errorf("Title should use gray color code, got: %q", title)
		}
		if !strings.Contains(title, ui.ColorReset) {
			t.Errorf("Title should include color reset code, got: %q", title)
		}
	}

	// Title should be indented
	if !strings.HasPrefix(title, "      ") {
		t.Errorf("Title should be indented with 6 spaces, got: %q", title)
	}
}

func TestBrowseItemRenderer_Title_PreviewTruncation(t *testing.T) {
	renderer := &browseItemRenderer{
		repo:           "owner/repo",
		prNumber:       123,
		collapsedFiles: make(map[string]bool),
	}

	tests := []struct {
		name           string
		body           string
		shouldTruncate bool
		shouldHaveEllipsis bool
	}{
		{
			name:           "short single line",
			body:           "Short comment",
			shouldTruncate: false,
			shouldHaveEllipsis: false,
		},
		{
			name:           "multi-line adds ellipsis",
			body:           "First line\nSecond line",
			shouldTruncate: false,
			shouldHaveEllipsis: true,
		},
		{
			name:           "very long line gets truncated",
			body:           strings.Repeat("a", 100),
			shouldTruncate: true,
			shouldHaveEllipsis: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := BrowseItem{
				Type:      "comment_preview",
				Path:      "src/main.go",
				IsPreview: true,
				Comment: &github.ReviewComment{
					ID:     123,
					Author: "reviewer",
					Body:   tt.body,
				},
			}

			title := renderer.Title(item)

			if tt.shouldTruncate {
				// Very long lines should be truncated to ~80 chars
				// Account for indentation and color codes
				plainText := strings.TrimPrefix(title, "      ")
				// Remove ANSI codes for length check
				var ansiRegex = regexp.MustCompile("\x1b\\[[0-9;]*m")
				plainText = ansiRegex.ReplaceAllString(plainText, "")
				if len(plainText) > 80 {
					t.Errorf("Title should be truncated, got length %d: %q", len(plainText), plainText)
				}
			}

			if tt.shouldHaveEllipsis {
				if !strings.Contains(title, "...") {
					t.Errorf("Title should contain ellipsis, got: %q", title)
				}
			}
		})
	}
}

func TestBuildCommentTree(t *testing.T) {
	comments := []*github.ReviewComment{
		{
			ID:     1,
			Path:   "file1.go",
			Line:   10,
			Author: "user1",
			Body:   "Comment on file1",
		},
		{
			ID:     2,
			Path:   "file2.go",
			Line:   20,
			Author: "user2",
			Body:   "Comment on file2",
		},
		{
			ID:     3,
			Path:   "file1.go",
			Line:   5,
			Author: "user3",
			Body:   "Earlier comment on file1",
		},
	}

	items := buildCommentTree(comments)

	// Should have: file1 header + 2 comments + 2 previews + file2 header + 1 comment + 1 preview
	// = 2 file headers + 3 comments + 3 previews = 8 items
	expectedCount := 8
	if len(items) != expectedCount {
		t.Errorf("buildCommentTree returned %d items, want %d", len(items), expectedCount)
	}

	// First item should be file1.go header (alphabetically first)
	if items[0].Type != "file" || items[0].Path != "file1.go" {
		t.Errorf("First item should be file1.go header, got type=%q path=%q", items[0].Type, items[0].Path)
	}

	// Comments within a file should be sorted by line number
	// file1.go has comments at lines 5 and 10, so line 5 should come first
	var file1Comments []BrowseItem
	for _, item := range items {
		if item.Path == "file1.go" && item.Type == "comment" {
			file1Comments = append(file1Comments, item)
		}
	}

	if len(file1Comments) != 2 {
		t.Fatalf("Expected 2 comments for file1.go, got %d", len(file1Comments))
	}

	if file1Comments[0].Comment.Line != 5 {
		t.Errorf("First comment in file1.go should be at line 5, got %d", file1Comments[0].Comment.Line)
	}

	if file1Comments[1].Comment.Line != 10 {
		t.Errorf("Second comment in file1.go should be at line 10, got %d", file1Comments[1].Comment.Line)
	}

	// Each comment should be followed by a preview
	for i, item := range items {
		if item.Type == "comment" && i+1 < len(items) {
			next := items[i+1]
			if next.Type != "comment_preview" {
				t.Errorf("Comment at index %d should be followed by preview, got %q", i, next.Type)
			}
			if next.Comment.ID != item.Comment.ID {
				t.Errorf("Preview should reference same comment, got IDs %d vs %d", next.Comment.ID, item.Comment.ID)
			}
		}
	}
}

func TestStripMarkdownForPreview(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text unchanged",
			input:    "This is plain text",
			expected: "This is plain text",
		},
		{
			name:     "removes image markdown",
			input:    "Text before ![alt text](https://example.com/image.png) text after",
			expected: "Text before  text after",
		},
		{
			name:     "converts link to text",
			input:    "Check out [this link](https://example.com) for more",
			expected: "Check out this link for more",
		},
		{
			name:     "handles multiple images and links",
			input:    "![img1](url1) and [link1](url2) and ![img2](url3)",
			expected: "and link1 and",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMarkdownForPreview(tt.input)
			if result != tt.expected {
				t.Errorf("stripMarkdownForPreview(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
