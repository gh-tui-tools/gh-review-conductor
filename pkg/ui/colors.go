package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/termenv"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

var colorEnabled = true

// SetColorEnabled toggles ANSI color output across the UI helpers.
func SetColorEnabled(enabled bool) {
	colorEnabled = enabled
	if !enabled {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// ColorsEnabled reports whether ANSI colors are enabled.
func ColorsEnabled() bool {
	return colorEnabled
}

// EmojiText returns emojiText when colors/emoji are enabled, otherwise the plain fallback.
func EmojiText(emojiText, plainText string) string {
	if !colorEnabled {
		return plainText
	}
	return emojiText
}

// Colorize applies ANSI color codes to text
func Colorize(color, text string) string {
	if !colorEnabled {
		return text
	}
	return color + text + ColorReset
}

// FormatDiffWithHeaders prepends git-style --- a/ and +++ b/ headers to a diff hunk.
// This is useful for displaying diff context with file path information.
func FormatDiffWithHeaders(diffHunk, path string) string {
	if path == "" {
		return diffHunk
	}
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n%s", path, path, diffHunk)
}

// TruncateDiff limits a diff to maxLines, appending "..." if truncated.
// If maxLines is 0 or negative, no truncation is applied.
func TruncateDiff(diff string, maxLines int) string {
	if maxLines <= 0 {
		return diff
	}
	lines := strings.Split(diff, "\n")
	if len(lines) <= maxLines {
		return diff
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
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
	if !colorEnabled {
		return text
	}
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

	if !colorEnabled {
		return strings.TrimSpace(text), nil
	}

	// Create a glamour renderer
	// Try auto-style first
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	// If auto-style fails, fallback to dark style
	if err != nil {
		r, err = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(80),
		)
	}
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
	isBot := strings.HasSuffix(author, "[bot]") || strings.EqualFold(author, "Copilot")
	name := author
	if strings.HasSuffix(author, "[bot]") {
		name = strings.TrimSuffix(author, "[bot]")
	}

	style := &AuthorStyle{
		Name:  name,
		IsBot: isBot,
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
	if includeIcon {
		icon := EmojiText("👤", "")
		if as.IsBot {
			icon = EmojiText("🤖", "")
		}
		if icon != "" {
			return fmt.Sprintf("%s %s", icon, Colorize(as.Color, "@"+as.Name))
		}
	}
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
		style.Emoji = "⚠️ " // Extra space after ⚠️ for better visual spacing
	}

	return style
}

// Format returns the formatted status string with color and emoji.
// When includeEmoji is true, the emoji indicator is prepended to the status.
func (ss *StatusStyle) Format(includeEmoji bool) string {
	if includeEmoji {
		emoji := EmojiText(ss.Emoji, "")
		if emoji != "" {
			return fmt.Sprintf("%s %s", emoji, Colorize(ss.Color, ss.Label))
		}
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
		parts = append(parts, Colorize(ColorYellow, EmojiText("⚠️ OUTDATED", "OUTDATED")))
	}

	parts = append(parts, rls.Status.Format(true))

	return strings.Join(parts, " ")
}

// FormatRelativeTime formats a time as a human-readable relative string
// like "5 minutes ago", "2 hours ago", "3 days ago", etc.
func FormatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}
