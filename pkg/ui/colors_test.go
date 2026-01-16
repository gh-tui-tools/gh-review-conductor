package ui

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDiffWithHeaders(t *testing.T) {
	tests := []struct {
		name     string
		diffHunk string
		path     string
		expected string
	}{
		{
			name:     "empty path returns hunk unchanged",
			diffHunk: "@@ -1,3 +1,4 @@\n context\n+added",
			path:     "",
			expected: "@@ -1,3 +1,4 @@\n context\n+added",
		},
		{
			name:     "adds git-style headers",
			diffHunk: "@@ -1,3 +1,4 @@\n context\n+added",
			path:     "file.go",
			expected: "--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n context\n+added",
		},
		{
			name:     "handles path with directories",
			diffHunk: "@@ -10,5 +10,7 @@",
			path:     "src/pkg/main.go",
			expected: "--- a/src/pkg/main.go\n+++ b/src/pkg/main.go\n@@ -10,5 +10,7 @@",
		},
		{
			name:     "empty hunk with path",
			diffHunk: "",
			path:     "file.go",
			expected: "--- a/file.go\n+++ b/file.go\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDiffWithHeaders(tt.diffHunk, tt.path)
			if result != tt.expected {
				t.Errorf("FormatDiffWithHeaders(%q, %q) = %q, want %q",
					tt.diffHunk, tt.path, result, tt.expected)
			}
		})
	}
}

func TestStripSuggestionBlock(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "no suggestion block",
			body:     "This is a regular comment",
			expected: "This is a regular comment",
		},
		{
			name:     "only suggestion block",
			body:     "```suggestion\nconst x = 1\n```",
			expected: "",
		},
		{
			name:     "text before suggestion",
			body:     "Here's a fix:\n```suggestion\nconst x = 1\n```",
			expected: "Here's a fix:",
		},
		{
			name:     "text after suggestion",
			body:     "```suggestion\nconst x = 1\n```\nWhat do you think?",
			expected: "What do you think?",
		},
		{
			name:     "text before and after suggestion",
			body:     "Try this:\n```suggestion\nconst x = 1\n```\nLet me know!",
			expected: "Try this:\n\nLet me know!",
		},
		{
			name:     "multiple suggestions",
			body:     "Option 1:\n```suggestion\na\n```\nOption 2:\n```suggestion\nb\n```",
			expected: "Option 1:\n\nOption 2:",
		},
		{
			name:     "suggestion with extra whitespace",
			body:     "  ```suggestion\ncode\n```  ",
			expected: "",
		},
		{
			name:     "removes markdown images",
			body:     "See this: ![screenshot](https://example.com/img.png)",
			expected: "See this:",
		},
		{
			name:     "removes images and suggestions",
			body:     "Look:\n![img](url)\n```suggestion\ncode\n```",
			expected: "Look:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripSuggestionBlock(tt.body)
			if result != tt.expected {
				t.Errorf("StripSuggestionBlock(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	defer func() { colorEnabled = originalEnabled }()

	tests := []struct {
		name         string
		colorEnabled bool
		color        string
		text         string
		shouldWrap   bool
	}{
		{
			name:         "colors enabled",
			colorEnabled: true,
			color:        ColorRed,
			text:         "error",
			shouldWrap:   true,
		},
		{
			name:         "colors disabled",
			colorEnabled: false,
			color:        ColorRed,
			text:         "error",
			shouldWrap:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			colorEnabled = tt.colorEnabled
			result := Colorize(tt.color, tt.text)

			if tt.shouldWrap {
				if !strings.HasPrefix(result, tt.color) {
					t.Errorf("Colorize() should start with color code when enabled")
				}
				if !strings.HasSuffix(result, ColorReset) {
					t.Errorf("Colorize() should end with reset code when enabled")
				}
				if !strings.Contains(result, tt.text) {
					t.Errorf("Colorize() should contain the text")
				}
			} else {
				if result != tt.text {
					t.Errorf("Colorize() = %q, want %q when colors disabled", result, tt.text)
				}
			}
		})
	}
}

func TestTruncateDiff(t *testing.T) {
	tests := []struct {
		name     string
		diff     string
		maxLines int
		expected string
	}{
		{
			name:     "no truncation needed",
			diff:     "line1\nline2\nline3",
			maxLines: 5,
			expected: "line1\nline2\nline3",
		},
		{
			name:     "exact limit",
			diff:     "line1\nline2\nline3",
			maxLines: 3,
			expected: "line1\nline2\nline3",
		},
		{
			name:     "truncation with ellipsis",
			diff:     "line1\nline2\nline3\nline4\nline5",
			maxLines: 3,
			expected: "line1\nline2\nline3\n...",
		},
		{
			name:     "zero maxLines means no truncation",
			diff:     "line1\nline2\nline3",
			maxLines: 0,
			expected: "line1\nline2\nline3",
		},
		{
			name:     "negative maxLines means no truncation",
			diff:     "line1\nline2",
			maxLines: -1,
			expected: "line1\nline2",
		},
		{
			name:     "single line no truncation",
			diff:     "single",
			maxLines: 1,
			expected: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateDiff(tt.diff, tt.maxLines)
			if result != tt.expected {
				t.Errorf("TruncateDiff(%q, %d) = %q, want %q", tt.diff, tt.maxLines, result, tt.expected)
			}
		})
	}
}

func TestColorizeDiff(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	defer func() { colorEnabled = originalEnabled }()

	colorEnabled = true

	diff := "+added line\n-removed line\n@@ hunk header\n context line"
	result := ColorizeDiff(diff)

	lines := strings.Split(result, "\n")
	if len(lines) != 4 {
		t.Fatalf("Expected 4 lines, got %d", len(lines))
	}

	// Check that each line starts with appropriate color
	if !strings.HasPrefix(lines[0], ColorGreen) {
		t.Errorf("Added line should start with green color")
	}
	if !strings.HasPrefix(lines[1], ColorRed) {
		t.Errorf("Removed line should start with red color")
	}
	if !strings.HasPrefix(lines[2], ColorCyan) {
		t.Errorf("Hunk header should start with cyan color")
	}
	if !strings.HasPrefix(lines[3], ColorGray) {
		t.Errorf("Context line should start with gray color")
	}
}

func TestCreateHyperlink(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	defer func() { colorEnabled = originalEnabled }()

	tests := []struct {
		name         string
		colorEnabled bool
		url          string
		text         string
		wantOSC8     bool
	}{
		{
			name:         "colors enabled with URL",
			colorEnabled: true,
			url:          "https://github.com",
			text:         "GitHub",
			wantOSC8:     true,
		},
		{
			name:         "colors disabled",
			colorEnabled: false,
			url:          "https://github.com",
			text:         "GitHub",
			wantOSC8:     false,
		},
		{
			name:         "empty URL",
			colorEnabled: true,
			url:          "",
			text:         "GitHub",
			wantOSC8:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			colorEnabled = tt.colorEnabled
			result := CreateHyperlink(tt.url, tt.text)

			if tt.wantOSC8 {
				// OSC8 hyperlinks start with \033]8;;
				if !strings.Contains(result, "\033]8;;") {
					t.Errorf("CreateHyperlink() should contain OSC8 escape sequence")
				}
				if !strings.Contains(result, tt.url) {
					t.Errorf("CreateHyperlink() should contain URL")
				}
				if !strings.Contains(result, tt.text) {
					t.Errorf("CreateHyperlink() should contain text")
				}
			} else {
				if result != tt.text {
					t.Errorf("CreateHyperlink() = %q, want %q", result, tt.text)
				}
			}
		})
	}
}

func TestRenderMarkdownCaching(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	defer func() {
		colorEnabled = originalEnabled
		// Note: We do NOT restore cachedMarkdownRenderer because sync.Once
		// has already run. Restoring it to nil would break subsequent tests.
	}()

	colorEnabled = true

	// First call should create or reuse the renderer
	result1, err := RenderMarkdown("**bold**")
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	if result1 == "" {
		t.Error("RenderMarkdown returned empty result")
	}

	// Capture the cached renderer
	firstRenderer := cachedMarkdownRenderer
	if firstRenderer == nil {
		t.Fatal("cachedMarkdownRenderer should be set after first call")
	}

	// Second call should reuse the same renderer
	result2, err := RenderMarkdown("_italic_")
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	if result2 == "" {
		t.Error("RenderMarkdown returned empty result")
	}

	// Verify the same renderer is used (not recreated)
	if cachedMarkdownRenderer != firstRenderer {
		t.Error("cachedMarkdownRenderer should be reused, not recreated")
	}
}

func TestRenderMarkdownEmptyInput(t *testing.T) {
	result, err := RenderMarkdown("")
	if err != nil {
		t.Errorf("RenderMarkdown(\"\") returned error: %v", err)
	}
	if result != "" {
		t.Errorf("RenderMarkdown(\"\") = %q, want empty string", result)
	}
}

func TestRenderMarkdownColorsDisabled(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	defer func() { colorEnabled = originalEnabled }()

	colorEnabled = false

	result, err := RenderMarkdown("**bold** text")
	if err != nil {
		t.Errorf("RenderMarkdown returned error: %v", err)
	}
	// When colors are disabled, should return trimmed plain text
	if result != "**bold** text" {
		t.Errorf("RenderMarkdown with colors disabled = %q, want %q", result, "**bold** text")
	}
}

func TestSetUIDebug(t *testing.T) {
	// Save original state and restore after test
	originalDebug := uiDebug.Load()
	defer func() { uiDebug.Store(originalDebug) }()

	// Test enabling debug
	SetUIDebug(true)
	if !uiDebug.Load() {
		t.Error("SetUIDebug(true) should set uiDebug to true")
	}

	// Test disabling debug
	SetUIDebug(false)
	if uiDebug.Load() {
		t.Error("SetUIDebug(false) should set uiDebug to false")
	}
}

func TestWarmupMarkdownRenderer(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	originalDebug := uiDebug.Load()
	defer func() {
		colorEnabled = originalEnabled
		uiDebug.Store(originalDebug)
	}()

	colorEnabled = true
	uiDebug.Store(false) // Disable debug to avoid stderr output

	// Call warmup - it runs in a goroutine and should not panic
	// Note: sync.Once means the renderer may already be initialized from other tests
	WarmupMarkdownRenderer()

	// Give the goroutine time to complete
	time.Sleep(100 * time.Millisecond)

	// After warmup (or if already initialized), the renderer should be cached
	// We can't reset sync.Once, so we just verify it doesn't panic and the
	// renderer is non-nil after the call
	if cachedMarkdownRenderer == nil {
		t.Error("cachedMarkdownRenderer should be initialized after WarmupMarkdownRenderer")
	}
}

func TestGetMarkdownRenderer(t *testing.T) {
	// Save original state and restore after test
	originalEnabled := colorEnabled
	originalDebug := uiDebug.Load()
	defer func() {
		colorEnabled = originalEnabled
		uiDebug.Store(originalDebug)
	}()

	colorEnabled = true
	uiDebug.Store(false) // Disable debug output

	// Note: sync.Once means the renderer may already be initialized from other tests
	// We test that getMarkdownRenderer returns a consistent non-nil value
	r := getMarkdownRenderer()
	if r == nil {
		t.Error("getMarkdownRenderer should return a non-nil renderer when colors are enabled")
	}

	// Calling again should return the same instance (cached)
	r2 := getMarkdownRenderer()
	if r2 != r {
		t.Error("getMarkdownRenderer should return the same cached instance")
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "zero time returns empty string",
			time:     time.Time{},
			expected: "",
		},
		{
			name:     "just now (seconds ago)",
			time:     now.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "1 minute ago",
			time:     now.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "multiple minutes ago",
			time:     now.Add(-45 * time.Minute),
			expected: "45 minutes ago",
		},
		{
			name:     "1 hour ago",
			time:     now.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "multiple hours ago",
			time:     now.Add(-10 * time.Hour),
			expected: "10 hours ago",
		},
		{
			name:     "1 day ago",
			time:     now.Add(-24 * time.Hour),
			expected: "1 day ago",
		},
		{
			name:     "multiple days ago",
			time:     now.Add(-5 * 24 * time.Hour),
			expected: "5 days ago",
		},
		{
			name:     "1 month ago",
			time:     now.Add(-35 * 24 * time.Hour),
			expected: "1 month ago",
		},
		{
			name:     "multiple months ago",
			time:     now.Add(-90 * 24 * time.Hour),
			expected: "3 months ago",
		},
		{
			name:     "1 year ago",
			time:     now.Add(-400 * 24 * time.Hour),
			expected: "1 year ago",
		},
		{
			name:     "multiple years ago",
			time:     now.Add(-800 * 24 * time.Hour),
			expected: "2 years ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRelativeTime(tt.time)
			if result != tt.expected {
				t.Errorf("FormatRelativeTime() = %q, want %q", result, tt.expected)
			}
		})
	}
}
