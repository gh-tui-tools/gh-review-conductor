package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/reflow/wordwrap"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

// Colorize applies ANSI color codes to text
func Colorize(color, text string) string {
	return color + text + ColorReset
}

// ColorizeDiff applies syntax highlighting to diff hunks
func ColorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var coloredLines []string

	for _, line := range lines {
		if len(line) == 0 {
			coloredLines = append(coloredLines, line)
			continue
		}

		switch line[0] {
		case '+':
			coloredLines = append(coloredLines, Colorize(ColorGreen, line))
		case '-':
			coloredLines = append(coloredLines, Colorize(ColorRed, line))
		case '@':
			coloredLines = append(coloredLines, Colorize(ColorCyan, line))
		default:
			coloredLines = append(coloredLines, Colorize(ColorGray, line))
		}
	}

	return strings.Join(coloredLines, "\n")
}

// ColorizeCode applies syntax highlighting to suggested code
func ColorizeCode(code string) string {
	return Colorize(ColorGreen, code)
}

// CreateHyperlink creates an OSC8 hyperlink
func CreateHyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// StripSuggestionBlock removes the suggestion code block and images from comment body
func StripSuggestionBlock(body string) string {
	result := strings.TrimSpace(body)

	// Remove ```suggestion...``` blocks
	suggestionRe := regexp.MustCompile("(?s)```suggestion\\s*\\n.*?```")
	result = suggestionRe.ReplaceAllString(result, "")

	// Remove markdown image links like ![alt](url)
	imageRe := regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	result = imageRe.ReplaceAllString(result, "")

	return strings.TrimSpace(result)
}

// WrapText wraps text to a maximum line width
func WrapText(text string, width int) string {
	return wordwrap.String(text, width)
}

// RenderMarkdown renders markdown text with glamour
func RenderMarkdown(text string) (string, error) {
	if text == "" {
		return "", nil
	}

	// Create a glamour renderer with dark style
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		// Fallback to plain text if rendering fails
		return text, nil
	}

	rendered, err := r.Render(text)
	if err != nil {
		// Fallback to plain text if rendering fails
		return text, nil
	}

	return strings.TrimSpace(rendered), nil
}

// ============================================================================
// Author Styling
// ============================================================================

// AuthorStyle represents styling information for a GitHub author.
type AuthorStyle struct {
	Name  string // Author name (without @ symbol)
	IsBot bool   // True if author name ends with [bot]
	Color string // ANSI color code (cyan for users, yellow for bots)
}

// NewAuthorStyle creates a new author style based on the author name.
// Bots (ending with [bot]) are colored yellow, regular users in cyan.
func NewAuthorStyle(author string) *AuthorStyle {
	style := &AuthorStyle{
		Name:  author,
		IsBot: strings.HasSuffix(author, "[bot]"),
	}

	if style.IsBot {
		style.Color = ColorYellow
	} else {
		style.Color = ColorCyan
	}

	return style
}

// Format returns the formatted author string with color (colored "@authorname").
func (as *AuthorStyle) Format(includeIcon bool) string {
	_ = includeIcon // Icon parameter unused currently
	return Colorize(as.Color, "@"+as.Name)
}

// ============================================================================
// Status Styling
// ============================================================================

// StatusStyle represents styling information for resolved/unresolved status.
type StatusStyle struct {
	IsResolved bool   // True if resolved, false if unresolved
	Label      string // "resolved" or "unresolved"
	Color      string // ANSI color code (green for resolved, yellow for unresolved)
	Emoji      string // Visual indicator (✅ or ⚠️)
}

// NewStatusStyle creates a new status style for the given resolved state.
func NewStatusStyle(isResolved bool) *StatusStyle {
	style := &StatusStyle{
		IsResolved: isResolved,
	}

	if isResolved {
		style.Label = "resolved"
		style.Color = ColorGreen
		style.Emoji = "✅"
	} else {
		style.Label = "unresolved"
		style.Color = ColorYellow
		style.Emoji = "⚠️"
	}

	return style
}

// Format returns the formatted status string with color and emoji.
// When includeEmoji is true, the emoji indicator is prepended to the status.
func (ss *StatusStyle) Format(includeEmoji bool) string {
	if includeEmoji {
		return fmt.Sprintf("%s %s", ss.Emoji, Colorize(ss.Color, ss.Label))
	}
	return Colorize(ss.Color, ss.Label)
}

// ============================================================================
// Review List Item Styling
// ============================================================================

// ReviewListStyle provides formatting for list items showing review comments or suggestions.
type ReviewListStyle struct {
	Author *AuthorStyle // Author styling (with bot detection)
	Status *StatusStyle // Resolution status styling
}

// NewReviewListStyle creates a new review list style from comment data.
func NewReviewListStyle(authorName string, isResolved bool) *ReviewListStyle {
	return &ReviewListStyle{
		Author: NewAuthorStyle(authorName),
		Status: NewStatusStyle(isResolved),
	}
}

// CommentListStyle is an alias for backward compatibility with resolve.go.
type CommentListStyle = ReviewListStyle

// NewCommentListStyle creates a new comment list style from comment data.
// Deprecated: Use NewReviewListStyle instead.
func NewCommentListStyle(authorName string, isResolved bool) *CommentListStyle {
	return NewReviewListStyle(authorName, isResolved)
}

// SuggestionListStyle is an alias for backward compatibility with applier.go.
type SuggestionListStyle = ReviewListStyle

// NewSuggestionListStyle creates a new suggestion list style from comment data.
// Deprecated: Use NewReviewListStyle instead.
func NewSuggestionListStyle(authorName string, isResolved bool) *SuggestionListStyle {
	return NewReviewListStyle(authorName, isResolved)
}

// FormatCommentTitle returns a formatted title for comment list display: "@author".
func (rls *ReviewListStyle) FormatCommentTitle(commentID int64) string {
	return rls.Author.Format(false)
}

// FormatCommentDescription returns a formatted description for comment list: "file:line [emoji status]".
func (rls *ReviewListStyle) FormatCommentDescription(filePath string, lineNumber int) string {
	return fmt.Sprintf("%s:%d %s", filePath, lineNumber, rls.Status.Format(true))
}

// FormatSuggestionTitle returns a formatted title for suggestion list: "@author • file:line".
func (rls *ReviewListStyle) FormatSuggestionTitle(filePath string, lineNumber int) string {
	return fmt.Sprintf("%s • %s:%d", rls.Author.Format(false), filePath, lineNumber)
}

// FormatSuggestionDescription returns a formatted description with status and tags for suggestion list.
func (rls *ReviewListStyle) FormatSuggestionDescription(hasSuggestion bool, isOutdated bool) string {
	var parts []string

	if hasSuggestion {
		parts = append(parts, "[suggestion]")
	}

	if isOutdated {
		parts = append(parts, Colorize(ColorYellow, "⚠️ OUTDATED"))
	}

	parts = append(parts, rls.Status.Format(true))

	return strings.Join(parts, " ")
}
