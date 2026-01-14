package ui

import (
	"strings"
	"testing"
)

func TestFormatBlockquote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: ">",
		},
		{
			name:     "single line",
			input:    "Hello world",
			expected: "> Hello world",
		},
		{
			name:     "multiple lines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "> Line 1\n> Line 2\n> Line 3",
		},
		{
			name:     "line with existing quote marker",
			input:    "> already quoted",
			expected: "> > already quoted",
		},
		{
			name:     "empty lines in middle",
			input:    "Before\n\nAfter",
			expected: "> Before\n> \n> After",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBlockquote(tt.input)
			if result != tt.expected {
				t.Errorf("FormatBlockquote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatQuotedReply(t *testing.T) {
	tests := []struct {
		name             string
		author           string
		body             string
		diffHunk         string
		path             string
		includeContext   bool
		checkContains    []string
		checkNotContains []string
	}{
		{
			name:           "basic quote without context",
			author:         "testuser",
			body:           "This is a comment",
			diffHunk:       "@@ -1,3 +1,4 @@\n context\n+added",
			path:           "file.go",
			includeContext: false,
			checkContains: []string{
				"> @testuser wrote:",
				"> This is a comment",
			},
			checkNotContains: []string{
				"```diff",
				"--- a/",
				"+++ b/",
			},
		},
		{
			name:           "quote with context",
			author:         "reviewer",
			body:           "Please fix this",
			diffHunk:       "@@ -10,5 +10,7 @@\n context line\n+new line",
			path:           "src/main.go",
			includeContext: true,
			checkContains: []string{
				"> ```diff",
				"> --- a/src/main.go",
				"> +++ b/src/main.go",
				"> @@ -10,5 +10,7 @@",
				"> ```",
				"> @reviewer wrote:",
				"> Please fix this",
			},
		},
		{
			name:           "empty diff hunk with context enabled",
			author:         "user",
			body:           "Comment body",
			diffHunk:       "",
			path:           "file.go",
			includeContext: true,
			checkContains: []string{
				"> @user wrote:",
				"> Comment body",
			},
			checkNotContains: []string{
				"```diff",
			},
		},
		{
			name:           "body with suggestion block stripped",
			author:         "reviewer",
			body:           "Here's a fix:\n```suggestion\nconst x = 1\n```",
			diffHunk:       "",
			path:           "",
			includeContext: false,
			checkContains: []string{
				"> @reviewer wrote:",
			},
			checkNotContains: []string{
				"```suggestion",
				"const x = 1",
			},
		},
		{
			name:           "multiline body",
			author:         "reviewer",
			body:           "Line 1\nLine 2\nLine 3",
			diffHunk:       "",
			path:           "",
			includeContext: false,
			checkContains: []string{
				"> @reviewer wrote:",
				"> Line 1",
				"> Line 2",
				"> Line 3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatQuotedReply(tt.author, tt.body, tt.diffHunk, tt.path, tt.includeContext)

			for _, expected := range tt.checkContains {
				if !strings.Contains(result, expected) {
					t.Errorf("FormatQuotedReply() missing expected content %q\nGot:\n%s", expected, result)
				}
			}

			for _, notExpected := range tt.checkNotContains {
				if strings.Contains(result, notExpected) {
					t.Errorf("FormatQuotedReply() should not contain %q\nGot:\n%s", notExpected, result)
				}
			}

			// Check that result ends with two empty lines for user's reply
			if !strings.HasSuffix(result, "\n\n") {
				t.Errorf("FormatQuotedReply() should end with two empty lines for user's reply")
			}
		})
	}
}

func TestFormatQuotedReplyStructure(t *testing.T) {
	// Test that context appears before author attribution
	result := FormatQuotedReply("user", "body", "@@ -1 +1 @@\n+line", "file.go", true)

	contextIdx := strings.Index(result, "```diff")
	authorIdx := strings.Index(result, "@user wrote:")

	if contextIdx == -1 {
		t.Fatal("Expected to find ```diff in result")
	}
	if authorIdx == -1 {
		t.Fatal("Expected to find @user wrote: in result")
	}
	if contextIdx >= authorIdx {
		t.Errorf("Context should appear before author attribution, but context at %d, author at %d", contextIdx, authorIdx)
	}
}
