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

		// Use interactive selector
		renderer := &browseCommentRenderer{repo: getRepoFromClient(client), prNumber: prNumber}
		selected, err := ui.SelectFromList(comments, renderer)
		if err != nil {
			return fmt.Errorf("selection cancelled: %w", err)
		}
		commentID = selected.ID
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

	if err := openCmd.Run(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	// Display confirmation
	commentLink := ui.CreateHyperlink(commentURL, fmt.Sprintf("Comment %d", commentID))
	fmt.Printf("%s Opened %s in your browser\n",
		ui.Colorize(ui.ColorGreen, "✓"),
		ui.Colorize(ui.ColorCyan, commentLink))

	return nil
}

// browseCommentRenderer implements ui.ItemRenderer for ReviewComments in browse context
type browseCommentRenderer struct {
	repo    string
	prNumber int
}

func (r *browseCommentRenderer) Title(comment *github.ReviewComment) string {
	style := ui.NewCommentListStyle(comment.Author, comment.IsResolved())
	return style.FormatCommentTitle(comment.ID)
}

func (r *browseCommentRenderer) Description(comment *github.ReviewComment) string {
	style := ui.NewCommentListStyle(comment.Author, comment.IsResolved())
	return style.FormatCommentDescription(comment.Path, comment.Line)
}

func (r *browseCommentRenderer) Preview(comment *github.ReviewComment) string {
	var preview strings.Builder
	maxLines := 20 // Limit preview to fit screen

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

	// Diff hunk/context (truncated with coloring) - only if substantial
	if comment.DiffHunk != "" && lines < maxLines {
		diffLines := strings.Split(comment.DiffHunk, "\n")
		// Only show context if it has more than just the header
		if len(diffLines) > 2 {
			preview.WriteString(ui.Colorize(ui.ColorCyan, "\n--- Context ---\n"))
			shown := 0
			for _, line := range diffLines {
				if lines >= maxLines-2 || shown >= 8 {
					preview.WriteString(ui.Colorize(ui.ColorGray, "...\n"))
					break
				}
				// Color diff lines
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

	// Thread replies (truncated)
	if len(comment.ThreadComments) > 0 && lines < maxLines {
		preview.WriteString("\n--- Replies ---\n")
		for i, threadComment := range comment.ThreadComments {
			if lines >= maxLines-1 {
				preview.WriteString("...\n")
				break
			}
			preview.WriteString(fmt.Sprintf("Reply %d by @%s:\n", i+1, threadComment.Author))
			lines++
			// Just first line of reply
			replyLines := strings.Split(threadComment.Body, "\n")
			if len(replyLines) > 0 && lines < maxLines {
				preview.WriteString(replyLines[0] + "\n")
				lines++
			}
		}
	}

	return preview.String()
}

func (r *browseCommentRenderer) EditPath(comment *github.ReviewComment) string {
	return comment.Path
}

func (r *browseCommentRenderer) EditLine(comment *github.ReviewComment) int {
	return comment.Line
}

func (r *browseCommentRenderer) BrowserURL(comment *github.ReviewComment) string {
	return comment.HTMLURL
}
