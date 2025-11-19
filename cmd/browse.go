package cmd

import (
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
		prNumber, err = client.GetCurrentBranchPR()
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

		// Create resolve action
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
			if item.Type == "comment" && collapsedFiles[item.Path] {
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
			return "SHOW_DETAIL", nil
		}

		selected, err := ui.SelectFromListWithAction(browseItems, renderer, resolveAction, "ctrl+r resolve", openAction, filterFunc, onSelect)
		if err != nil {
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
		prNumber, err = client.GetCurrentBranchPR()
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
	Type    string // "file" or "comment"
	Path    string
	Comment *github.ReviewComment
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
			items = append(items, BrowseItem{
				Type:    "comment",
				Path:    path,
				Comment: c,
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
		if r.collapsedFiles != nil && r.collapsedFiles[item.Path] {
			icon = "▶"
		}
		return ui.Colorize(ui.ColorCyan, fmt.Sprintf("%s 📂 %s", icon, item.Path))
	}
	// Comment
	style := ui.NewReviewListStyle(item.Comment.Author, item.Comment.IsResolved())
	// Indent with tree structure
	return "  └── " + style.FormatCommentTitle(item.Comment.ID)
}

func (r *browseItemRenderer) Description(item BrowseItem) string {
	if item.Type == "file" {
		return ""
	}
	// Comment
	style := ui.NewReviewListStyle(item.Comment.Author, item.Comment.IsResolved())
	// Show line number and status, but not path (since it's under the file header)
	return fmt.Sprintf("Line %d %s", item.Comment.Line, style.Status.Format(true))
}

func (r *browseItemRenderer) Preview(item BrowseItem) string {
	if item.Type == "file" {
		return fmt.Sprintf("File: %s\n\nSelect a comment below to view details.", item.Path)
	}
	
	// Reuse the logic from browseCommentRenderer but adapted for BrowseItem
	comment := item.Comment
	var preview strings.Builder
	maxLines := 20 

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

	if comment.IsOutdated {
		preview.WriteString(ui.Colorize(ui.ColorYellow, "⚠️  OUTDATED\n"))
	}

	lines := strings.Count(preview.String(), "\n") + 1

	// Comment body (truncated)
	body := ui.StripSuggestionBlock(comment.Body)
	if body != "" && lines < maxLines {
		preview.WriteString("\n--- Comment ---\n")
		bodyLines := strings.Split(body, "\n")
		for _, line := range bodyLines {
			if lines >= maxLines-2 {
				preview.WriteString("...\n")
				break
			}
			preview.WriteString(line + "\n")
			lines++
		}
	}

	// Diff hunk/context (truncated with coloring)
	if comment.DiffHunk != "" && lines < maxLines {
		diffLines := strings.Split(comment.DiffHunk, "\n")
		if len(diffLines) > 2 {
			preview.WriteString(ui.Colorize(ui.ColorCyan, "\n--- Context ---\n"))
			shown := 0
			for _, line := range diffLines {
				if lines >= maxLines-2 || shown >= 8 {
					preview.WriteString(ui.Colorize(ui.ColorGray, "...\n"))
					break
				}
				coloredLine := line
				if len(line) > 0 {
					switch line[0] {
					case '+':
						coloredLine = ui.Colorize(ui.ColorGreen, line)
					case '-':
						coloredLine = ui.Colorize(ui.ColorRed, line)
					case '@':
						coloredLine = ui.Colorize(ui.ColorCyan, line)
					default:
						coloredLine = ui.Colorize(ui.ColorGray, line)
					}
				}
				preview.WriteString(coloredLine + "\n")
				lines++
				shown++
			}
		}
	}

	// Thread replies
	if len(comment.ThreadComments) > 0 && lines < maxLines {
		preview.WriteString("\n--- Replies ---\n")
		for i, threadComment := range comment.ThreadComments {
			if lines >= maxLines-1 {
				preview.WriteString("...\n")
				break
			}
			preview.WriteString(fmt.Sprintf("Reply %d by @%s:\n", i+1, threadComment.Author))
			lines++
			replyLines := strings.Split(threadComment.Body, "\n")
			if len(replyLines) > 0 && lines < maxLines {
				preview.WriteString(replyLines[0] + "\n")
				lines++
			}
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
