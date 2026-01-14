package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/chmouel/gh-prreview/pkg/github"
	"github.com/chmouel/gh-prreview/pkg/ui"
	"github.com/spf13/cobra"
)

var browseDebug bool

var browseCmd = &cobra.Command{
	Use:   "browse [PR_NUMBER] [COMMENT_ID]",
	Short: "Browse and open review comments in your browser",
	Long: `Browse and open GitHub pull request review comments in your default browser.

When no arguments are provided, PR is inferred from the current branch and you can interactively select a comment.
When one argument is provided, it's treated as COMMENT_ID and PR is inferred from the current branch.
When two arguments are provided, the first is PR_NUMBER and the second is COMMENT_ID.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runBrowse,
}

func init() {
	browseCmd.Flags().BoolVar(&browseDebug, "debug", false, "Enable debug output")
}

func runBrowse(cmd *cobra.Command, args []string) error {
	client := github.NewClient()
	client.SetDebug(browseDebug)
	if repoFlag != "" {
		client.SetRepo(repoFlag)
	}

	var prNumber int
	var commentID int64
	var err error

	// Parse arguments based on count
	if len(args) == 0 {
		// No args: infer PR and let user select a comment interactively
		prNumber, err = getPRNumberWithSelection([]string{}, client)
		if err != nil {
			return err
		}

		comments, err := client.FetchReviewComments(prNumber)
		if err != nil {
			return fmt.Errorf("failed to fetch review comments: %w", err)
		}
		if len(comments) == 0 {
			fmt.Printf("No review comments found in %s\n",
				ui.CreateHyperlink(fmt.Sprintf("https://github.com/%s/pull/%d", getRepoFromClient(client), prNumber),
					ui.Colorize(ui.ColorCyan, fmt.Sprintf("PR #%d", prNumber))))
			return nil
		}

		// Track collapsed state
		collapsedFiles := make(map[string]bool)

		// Use interactive selector with resolve action
		renderer := &browseItemRenderer{
			repo:           getRepoFromClient(client),
			prNumber:       prNumber,
			collapsedFiles: collapsedFiles,
		}

		// Convert comments to tree structure
		browseItems := buildCommentTree(comments)

		// Create resolve actions
		resolveAction := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", nil // Cannot resolve a file header
			}
			return resolveCommentAction(client, prNumber, item.Comment)
		}

		// Create open action (on 'o')
		openAction := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", nil // Cannot open a file header
			}
			if err := openCommentInBrowser(client, prNumber, item.Comment.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("Opened comment %d in browser", item.Comment.ID), nil
		}

		// Filter function (hide resolved and collapsed)
		filterFunc := func(item BrowseItem, hideResolved bool) bool {
			// 1. Check collapse state (Always applies)
			if (item.Type == "comment" || item.Type == "comment_preview") && collapsedFiles[item.Path] {
				return false
			}

			// 2. Check resolved state (Only if hideResolved is true)
			if hideResolved {
				if item.Type == "file" {
					return true // Always show headers
				}
				return !item.Comment.IsResolved()
			}

			return true
		}

		// Handle selection (Enter key)
		onSelect := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				collapsedFiles[item.Path] = !collapsedFiles[item.Path]
				return "", nil // Just refresh
			}

			// Refresh thread comments before showing details to ensure fresh data
			freshComments, err := client.FetchReviewComments(prNumber)
			if err != nil {
				// Show warning but still display detail with existing data
				return "SHOW_DETAIL:failed to refresh comments: " + err.Error(), nil
			}
			for _, fresh := range freshComments {
				if fresh.ID == item.Comment.ID {
					item.Comment.ThreadComments = fresh.ThreadComments
					item.Comment.SubjectType = fresh.SubjectType // Also refresh resolved status
					break
				}
			}

			return "SHOW_DETAIL", nil
		}

		// Editor actions for R (resolve with comment)
		editorPrepareR := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", fmt.Errorf("cannot add comment to file header")
			}
			if item.Comment.ThreadID == "" {
				return "", fmt.Errorf("comment has no thread ID")
			}
			return "", nil // No initial content for resolve+comment
		}

		editorCompleteR := func(item BrowseItem, body string) (string, error) {
			comment := item.Comment
			reply, err := client.ReplyToReviewComment(prNumber, comment.ID, body)
			if err != nil {
				return "", fmt.Errorf("failed to add comment: %w", err)
			}

			// Add reply to local thread so it shows in details view
			comment.ThreadComments = append(comment.ThreadComments, *reply)

			// Toggle resolved state
			statusMsg, err := resolveCommentAction(client, prNumber, comment)
			if err != nil {
				return "", err
			}

			if reply != nil && reply.HTMLURL != "" {
				link := ui.CreateHyperlink(reply.HTMLURL, reply.HTMLURL)
				return fmt.Sprintf("%s\nPosted a comment to:\n%s", statusMsg, link), nil
			}

			return statusMsg, nil
		}

		// Editor actions for Q (quote reply without context)
		editorPrepareQ := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", fmt.Errorf("cannot quote reply to file header")
			}
			comment := item.Comment
			return ui.FormatQuotedReply(
				comment.Author,
				comment.Body,
				comment.DiffHunk,
				comment.Path,
				false, // Don't include context
			), nil
		}

		editorCompleteQ := func(item BrowseItem, body string) (string, error) {
			comment := item.Comment
			reply, err := client.ReplyToReviewComment(prNumber, comment.ID, body)
			if err != nil {
				return "", fmt.Errorf("failed to post reply: %w", err)
			}

			// Add reply to local thread so it shows in details view
			comment.ThreadComments = append(comment.ThreadComments, *reply)

			url := reply.HTMLURL
			if url == "" {
				return fmt.Sprintf("Posted comment %d", reply.ID), nil
			}

			// Show clickable hyperlink with full URL visible
			link := ui.CreateHyperlink(url, url)
			return fmt.Sprintf("Posted a comment to:\n%s", link), nil
		}

		// Editor actions for C (quote reply with context)
		editorPrepareC := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", fmt.Errorf("cannot quote reply to file header")
			}
			comment := item.Comment
			return ui.FormatQuotedReply(
				comment.Author,
				comment.Body,
				comment.DiffHunk,
				comment.Path,
				true, // Include context
			), nil
		}

		// editorCompleteC is the same as editorCompleteQ - just post the reply
		editorCompleteC := editorCompleteQ

		// Callback to check if an item is resolved (for dynamic help text)
		isItemResolved := func(item BrowseItem) bool {
			if item.Type == "file" {
				return false
			}
			return item.Comment.IsResolved()
		}

		// Callback to refresh items from the API
		refreshItems := func() ([]BrowseItem, error) {
			freshComments, err := client.FetchReviewComments(prNumber)
			if err != nil {
				return nil, err
			}
			return buildCommentTree(freshComments), nil
		}

		// Agent action - launch coding agent with comment details
		agentAction := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", fmt.Errorf("cannot launch agent on file header")
			}
			prompt := fmt.Sprintf("Review comment on %s:%d\n\n%s",
				item.Comment.Path,
				item.Comment.Line,
				item.Comment.Body)
			return "LAUNCH_AGENT:" + prompt, nil
		}

		// Edit action - open file in editor at comment line
		editAction := func(item BrowseItem) (string, error) {
			if item.Type == "file" {
				return "", fmt.Errorf("cannot edit file header")
			}
			return fmt.Sprintf("EDIT_FILE:%s:%d", item.Comment.Path, item.Comment.Line), nil
		}

		selected, err := ui.Select(ui.SelectorOptions[BrowseItem]{
			Items:    browseItems,
			Renderer: renderer,

			// Core callbacks
			OnSelect:       onSelect,
			OnOpen:         openAction,
			FilterFunc:     filterFunc,
			IsItemResolved: isItemResolved,
			RefreshItems:   refreshItems,

			// r/u key: resolve/unresolve
			ResolveAction: resolveAction,
			ResolveKey:    "r resolve",
			ResolveKeyAlt: "u unresolve",

			// R/U key: resolve+comment via editor
			ResolveCommentPrepare:  editorPrepareR,
			ResolveCommentComplete: editorCompleteR,
			ResolveCommentKey:      "R resolve+comment",
			ResolveCommentKeyAlt:   "U unresolve+comment",

			// Q key: quote reply via editor
			QuotePrepare:  editorPrepareQ,
			QuoteComplete: editorCompleteQ,
			QuoteKey:      "Q quote",

			// C key: quote+context via editor
			QuoteContextPrepare:  editorPrepareC,
			QuoteContextComplete: editorCompleteC,
			QuoteContextKey:      "C quote+context",

			// a key: launch coding agent
			AgentAction: agentAction,
			AgentKey:    "a agent",

			// e key: edit file
			EditAction: editAction,
			EditKey:    "e edit",
		})
		if err != nil {
			if errors.Is(err, ui.ErrNoSelection) {
				return nil
			}
			return fmt.Errorf("selection cancelled: %w", err)
		}

		if selected.Type == "file" {
			// If they selected a header and quit (enter), maybe just do nothing or open the file?
			// For now, let's assume they meant to select a comment.
			// But since we return on Enter, we need to handle it.
			// Let's just print a message.
			fmt.Println("Selected a file header. Please select a comment.")
			return nil
		}

		commentID = selected.Comment.ID
	} else if len(args) == 1 {
		// One argument: treat as COMMENT_ID, infer PR from current branch
		commentID, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid comment ID: %s", args[0])
		}
		prNumber, err = getPRNumberWithSelection([]string{}, client)
		if err != nil {
			return err
		}
	} else if len(args) == 2 {
		// Two arguments: first is PR, second is COMMENT_ID
		prNumber, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid PR number: %s", args[0])
		}
		commentID, err = strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid comment ID: %s", args[1])
		}
	}

	// Open comment in browser
	return openCommentInBrowser(client, prNumber, commentID)
}

func openCommentInBrowser(client *github.Client, prNumber int, commentID int64) error {
	// Fetch review comments to find the comment URL
	comments, err := client.FetchReviewComments(prNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch review comments: %w", err)
	}

	// Find the comment with the given ID
	var commentURL string
	for _, comment := range comments {
		if comment.ID == commentID {
			commentURL = comment.HTMLURL
			break
		}
	}

	if commentURL == "" {
		return fmt.Errorf("comment ID %d not found in PR #%d", commentID, prNumber)
	}

	// Open in browser (OS-agnostic)
	var openCmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS
		openCmd = exec.Command("open", commentURL)
	case "linux":
		// Linux
		openCmd = exec.Command("xdg-open", commentURL)
	case "windows":
		// Windows
		openCmd = exec.Command("cmd", "/c", "start", commentURL)
	default:
		// Fallback: try xdg-open
		openCmd = exec.Command("xdg-open", commentURL)
	}

	if err := openCmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	return nil
}

// BrowseItem represents an item in the browse list (either a file header or a comment)
type BrowseItem struct {
	Type      string // "file", "comment", "comment_preview"
	Path      string
	Comment   *github.ReviewComment
	IsPreview bool
}

// buildCommentTree converts a flat list of comments into a tree-like structure
func buildCommentTree(comments []*github.ReviewComment) []BrowseItem {
	// Sort comments by Path then Line
	// We need a stable sort for the tree structure
	// Make a copy to avoid modifying original slice if needed
	sortedComments := make([]*github.ReviewComment, len(comments))
	copy(sortedComments, comments)

	// Simple bubble sort or similar isn't needed, just use standard sort with custom comparator
	// But we need to import sort. Let's do it manually or add import.
	// Since I can't easily add imports without context, I'll assume sort is available or use a simple swap.
	// Actually, let's just use a simple grouping logic.

	// Group by file
	files := make(map[string][]*github.ReviewComment)
	var filePaths []string

	for _, c := range comments {
		if _, exists := files[c.Path]; !exists {
			filePaths = append(filePaths, c.Path)
		}
		files[c.Path] = append(files[c.Path], c)
	}

	// Sort file paths
	// We need to sort strings. I'll implement a simple string sort since I can't see imports easily.
	for i := 0; i < len(filePaths); i++ {
		for j := i + 1; j < len(filePaths); j++ {
			if filePaths[i] > filePaths[j] {
				filePaths[i], filePaths[j] = filePaths[j], filePaths[i]
			}
		}
	}

	var items []BrowseItem

	for _, path := range filePaths {
		// Add File Header
		items = append(items, BrowseItem{
			Type: "file",
			Path: path,
		})

		// Sort comments in this file by line
		fileComments := files[path]
		for i := 0; i < len(fileComments); i++ {
			for j := i + 1; j < len(fileComments); j++ {
				if fileComments[i].Line > fileComments[j].Line {
					fileComments[i], fileComments[j] = fileComments[j], fileComments[i]
				}
			}
		}

		// Add Comments
		for _, c := range fileComments {
			// Main comment item
			items = append(items, BrowseItem{
				Type:    "comment",
				Path:    path,
				Comment: c,
			})
			// Preview item (skippable)
			items = append(items, BrowseItem{
				Type:      "comment_preview",
				Path:      path,
				Comment:   c,
				IsPreview: true,
			})
		}
	}

	return items
}

// browseItemRenderer implements ui.ItemRenderer for BrowseItem
type browseItemRenderer struct {
	repo           string
	prNumber       int
	collapsedFiles map[string]bool
}

func (r *browseItemRenderer) Title(item BrowseItem) string {
	if item.Type == "file" {
		icon := "▼"
		collapsedIcon := "▶"
		folder := "📂"
		if !ui.ColorsEnabled() {
			icon = "-"
			collapsedIcon = "+"
			folder = ""
		}
		if r.collapsedFiles != nil && r.collapsedFiles[item.Path] {
			icon = collapsedIcon
		}
		title := fmt.Sprintf("%s %s", icon, item.Path)
		if folder != "" {
			title = fmt.Sprintf("%s %s %s", icon, folder, item.Path)
		}
		return ui.Colorize(ui.ColorCyan, strings.TrimSpace(title))
	}

	if item.IsPreview {
		// Show truncated body for preview item
		body := ui.StripSuggestionBlock(item.Comment.Body)
		lines := strings.Split(body, "\n")
		preview := "..."
		if len(lines) > 0 {
			preview = lines[0]
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			} else if len(lines) > 1 {
				preview += "..."
			}
		}
		return "      " + ui.Colorize(ui.ColorGray, preview)
	}

	// Comment Metadata
	style := ui.NewReviewListStyle(item.Comment.Author, item.Comment.IsResolved())
	// Indent with tree structure
	return fmt.Sprintf("  └── %s Line %d %s", style.FormatCommentTitle(item.Comment.ID), item.Comment.Line, style.Status.Format(true))
}

func (r *browseItemRenderer) Description(item BrowseItem) string {
	return ""
}

func (r *browseItemRenderer) Preview(item BrowseItem) string {
	if item.Type == "file" {
		return fmt.Sprintf("File: %s\n\nSelect a comment below to view details.", item.Path)
	}

	// Reuse the logic from browseCommentRenderer but adapted for BrowseItem
	comment := item.Comment
	var preview strings.Builder

	// Header
	status := "unresolved"
	statusColor := ui.ColorYellow
	if comment.IsResolved() {
		status = "resolved"
		statusColor = ui.ColorGreen
	}
	preview.WriteString(ui.Colorize(ui.ColorCyan, fmt.Sprintf("Author: @%s\n", comment.Author)))
	preview.WriteString(ui.Colorize(ui.ColorCyan, fmt.Sprintf("Location: %s:%d\n", comment.Path, comment.Line)))
	preview.WriteString(ui.Colorize(ui.ColorCyan, fmt.Sprintf("Status: %s\n", ui.Colorize(statusColor, status))))
	if comment.HTMLURL != "" {
		preview.WriteString(ui.Colorize(ui.ColorCyan, fmt.Sprintf("URL: %s\n", ui.CreateHyperlink(comment.HTMLURL, comment.HTMLURL))))
	}
	if !comment.CreatedAt.IsZero() {
		preview.WriteString(ui.Colorize(ui.ColorCyan, fmt.Sprintf("Time: %s\n", ui.FormatRelativeTime(comment.CreatedAt))))
	}

	if comment.IsOutdated {
		preview.WriteString(ui.Colorize(ui.ColorYellow, ui.EmojiText("⚠️  OUTDATED\n", "OUTDATED\n")))
	}

	// Comment body (with markdown rendering, truncated to first 200 lines of source)
	body := ui.StripSuggestionBlock(comment.Body)
	if body != "" {
		preview.WriteString("\n--- Comment ---\n")

		// Truncate very long comments before rendering to avoid slowness
		bodyLines := strings.Split(body, "\n")
		if len(bodyLines) > 200 {
			body = strings.Join(bodyLines[:200], "\n") + "\n\n...(truncated, content too long)"
		}

		// Try to render markdown
		rendered, err := ui.RenderMarkdown(body)
		if err == nil && rendered != "" {
			preview.WriteString(rendered)
		} else {
			// Fallback to wrapped text
			preview.WriteString(ui.WrapText(body, 80))
		}
		preview.WriteString("\n")
	}

	// Suggested code (with syntax highlighting based on file type)
	if comment.HasSuggestion && comment.SuggestedCode != "" {
		preview.WriteString(ui.Colorize(ui.ColorCyan, "\n--- Suggested Code ---\n"))
		lang := ui.CodeFenceLanguageFromPath(comment.Path)
		md := fmt.Sprintf("```%s\n%s\n```", lang, comment.SuggestedCode)
		if rendered, err := ui.RenderMarkdown(md); err == nil && rendered != "" {
			preview.WriteString(rendered)
		} else {
			preview.WriteString(ui.Colorize(ui.ColorGreen, comment.SuggestedCode))
		}
		preview.WriteString("\n")
	}

	// Diff hunk/context (with coloring, limited to 8 lines for relevance)
	if comment.DiffHunk != "" {
		diffLines := strings.Split(comment.DiffHunk, "\n")
		if len(diffLines) > 2 {
			preview.WriteString(ui.Colorize(ui.ColorCyan, "\n--- Context ---\n"))
			truncated := ui.TruncateDiff(comment.DiffHunk, 8)
			preview.WriteString(ui.ColorizeDiff(truncated))
			preview.WriteString("\n")
		}
	}

	// Thread replies (with markdown rendering, truncated to first 100 lines each)
	if len(comment.ThreadComments) > 0 {
		preview.WriteString("\n--- Replies ---\n")
		for i, threadComment := range comment.ThreadComments {
			// Add vertical spacing before each reply
			preview.WriteString("\n")
			// Format: Reply N by @author | URL | time ago
			replyHeader := fmt.Sprintf("Reply %d by @%s", i+1, threadComment.Author)
			if threadComment.HTMLURL != "" {
				replyHeader += fmt.Sprintf(" | %s", ui.CreateHyperlink(threadComment.HTMLURL, threadComment.HTMLURL))
			}
			if !threadComment.CreatedAt.IsZero() {
				replyHeader += fmt.Sprintf(" | %s", ui.FormatRelativeTime(threadComment.CreatedAt))
			}
			preview.WriteString(replyHeader + "\n")

			// Truncate very long replies before rendering to avoid slowness
			replyBody := threadComment.Body
			replyLines := strings.Split(replyBody, "\n")
			if len(replyLines) > 100 {
				replyBody = strings.Join(replyLines[:100], "\n") + "\n\n...(truncated, content too long)"
			}

			// Render reply body with markdown
			rendered, err := ui.RenderMarkdown(replyBody)
			if err == nil && rendered != "" {
				preview.WriteString(rendered)
			} else {
				preview.WriteString(ui.WrapText(replyBody, 80))
			}
			preview.WriteString("\n")
		}
	}

	return preview.String()
}

func (r *browseItemRenderer) EditPath(item BrowseItem) string {
	return item.Path
}

func (r *browseItemRenderer) EditLine(item BrowseItem) int {
	if item.Type == "file" {
		return 0
	}
	return item.Comment.Line
}

func (r *browseItemRenderer) FilterValue(item BrowseItem) string {
	if item.Type == "file" {
		return item.Path
	}
	return item.Path + " " + r.Title(item) + " " + r.Description(item) + " " + item.Comment.Body
}

func (r *browseItemRenderer) IsSkippable(item BrowseItem) bool {
	return item.IsPreview
}

// resolveCommentAction resolves a review comment thread
func resolveCommentAction(client *github.Client, prNumber int, comment *github.ReviewComment) (string, error) {
	if comment.ThreadID == "" {
		return "", fmt.Errorf("comment has no thread ID")
	}

	if comment.IsResolved() {
		// Unresolve
		if err := client.UnresolveThread(comment.ThreadID); err != nil {
			return "", err
		}
		comment.SubjectType = "line" // Reset to default
		return "Marked as unresolved", nil
	} else {
		// Resolve
		if err := client.ResolveThread(comment.ThreadID); err != nil {
			return "", err
		}
		comment.SubjectType = "resolved"
		return "Marked as resolved", nil
	}
}
