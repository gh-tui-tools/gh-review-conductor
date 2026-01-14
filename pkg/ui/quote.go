package ui

import (
	"strings"
)

// FormatBlockquote formats text as a GitHub markdown blockquote.
// Each line is prefixed with "> ".
func FormatBlockquote(text string) string {
	if text == "" {
		return ">"
	}
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, "> "+line)
	}
	return strings.Join(result, "\n")
}

// FormatQuotedReply formats a review comment as a blockquote for replying.
// If includeContext is true, it includes the diff hunk as a quoted code block
// above the author attribution, with the file path.
func FormatQuotedReply(author, body, diffHunk, path string, includeContext bool) string {
	var parts []string

	// Optionally add code context first (above the author line)
	if includeContext && diffHunk != "" {
		// Format the diff hunk with git-style headers
		formattedDiff := FormatDiffWithHeaders(diffHunk, path)
		// Wrap in a quoted code fence
		parts = append(parts, "> ```diff")
		for _, line := range strings.Split(formattedDiff, "\n") {
			parts = append(parts, "> "+line)
		}
		parts = append(parts, "> ```")
		parts = append(parts, ">") // Empty blockquote line for spacing
	}

	// Add author attribution
	parts = append(parts, FormatBlockquote("@"+author+" wrote:"))
	parts = append(parts, ">") // Empty blockquote line for spacing

	// Add comment body (strip suggestion blocks for cleaner quoting)
	cleanBody := StripSuggestionBlock(body)
	if cleanBody != "" {
		parts = append(parts, FormatBlockquote(cleanBody))
	}

	// Add empty lines for user's reply
	parts = append(parts, "")
	parts = append(parts, "")

	return strings.Join(parts, "\n")
}
