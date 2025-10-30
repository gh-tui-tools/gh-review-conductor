package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chmouel/gh-prreview/pkg/github"
	"github.com/chmouel/gh-prreview/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	resolveUnresolve bool
	resolveDebug     bool
	resolveAll       bool
)

var resolveCmd = &cobra.Command{
	Use:   "resolve [COMMENT_ID] or [PR_NUMBER] [COMMENT_ID]",
	Short: "Resolve or unresolve review comment threads",
	Long: `Mark review comment threads as resolved or unresolved. Use --all to apply the action to all unresolved comments on a PR.
When no arguments are provided, PR is inferred from the current branch and you will be prompted for a comment ID.
When one argument is provided, it's treated as COMMENT_ID and PR is inferred from the current branch.
When two arguments are provided, the first is PR_NUMBER and the second is COMMENT_ID.`,
	Args: cobra.MinimumNArgs(0),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().BoolVar(&resolveUnresolve, "unresolve", false, "Mark the thread as unresolved instead of resolved")
	resolveCmd.Flags().BoolVar(&resolveDebug, "debug", false, "Enable debug output")
	resolveCmd.Flags().BoolVar(&resolveAll, "all", false, "Apply action to all unresolved comments on the PR")
}

func runResolve(cmd *cobra.Command, args []string) error {
	client := github.NewClient()
	client.SetDebug(resolveDebug)
	if repoFlag != "" {
		client.SetRepo(repoFlag)
	}

	var prNumber int
	var commentID int64
	var err error

	// Parse arguments based on count
	if len(args) == 0 {
		// No args: infer PR and prompt for comment ID
		prNumber, err = client.GetCurrentBranchPR()
		if err != nil {
			return err
		}

		// Prompt for comment ID
		fmt.Printf("Enter comment ID: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		commentID, err = strconv.ParseInt(strings.TrimSpace(input), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid comment ID: %s", strings.TrimSpace(input))
		}
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
	} else {
		return fmt.Errorf("too many arguments provided")
	}

	// Handle --all flag
	if resolveAll {
		return resolveAllComments(client, prNumber)
	}

	// Handle individual comment resolution
	if commentID == 0 {
		return fmt.Errorf("no comment selected")
	}

	return resolveIndividualComment(client, prNumber, commentID)
}

func resolveAllComments(client *github.Client, prNumber int) error {
	// Fetch all review comments
	comments, err := client.FetchReviewComments(prNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch review comments: %w", err)
	}

	// Filter unresolved comments
	var unresolvedComments []*github.ReviewComment
	for _, comment := range comments {
		if !comment.IsResolved() {
			unresolvedComments = append(unresolvedComments, comment)
		}
	}

	if len(unresolvedComments) == 0 {
		fmt.Printf("No unresolved comments found in %s\n",
			ui.CreateHyperlink(fmt.Sprintf("https://github.com/%s/pull/%d", getRepoFromClient(client), prNumber),
				ui.Colorize(ui.ColorCyan, fmt.Sprintf("PR #%d", prNumber))))
		return nil
	}

	// Show summary and ask for confirmation
	prLink := ui.CreateHyperlink(fmt.Sprintf("https://github.com/%s/pull/%d", getRepoFromClient(client), prNumber),
		ui.Colorize(ui.ColorCyan, fmt.Sprintf("PR #%d", prNumber)))
	fmt.Printf("Found %s unresolved comment(s) in %s:\n",
		ui.Colorize(ui.ColorYellow, fmt.Sprintf("%d", len(unresolvedComments))), prLink)

	for _, comment := range unresolvedComments {
		// Create clickable link to the review comment
		fileLocation := fmt.Sprintf("%s:%d", comment.Path, comment.Line)
		clickableLocation := ui.CreateHyperlink(comment.HTMLURL, fileLocation)

		// Truncate comment body and colorize it
		commentPreview := truncateString(ui.StripSuggestionBlock(comment.Body), 50)
		if commentPreview == "" {
			commentPreview = "(no text content)"
		}

		fmt.Printf("  • %s: %s (%s)\n",
			ui.Colorize(ui.ColorCyan, fmt.Sprintf("Comment %d", comment.ID)),
			ui.Colorize(ui.ColorGray, commentPreview),
			ui.Colorize(ui.ColorGreen, clickableLocation))
	}

	action := "resolve"
	actionColor := ui.ColorGreen
	if resolveUnresolve {
		action = "unresolve"
		actionColor = ui.ColorYellow
	}

	fmt.Printf("\n%s all %s comment(s)? [y/N]: ",
		ui.Colorize(actionColor, fmt.Sprintf("Are you sure you want to %s", action)),
		ui.Colorize(ui.ColorYellow, fmt.Sprintf("%d", len(unresolvedComments))))
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		fmt.Println(ui.Colorize(ui.ColorGray, "Operation cancelled"))
		return nil
	}

	// Resolve/unresolve all comments
	successCount := 0
	errorCount := 0

	for _, comment := range unresolvedComments {
		commentLink := ui.CreateHyperlink(comment.HTMLURL, fmt.Sprintf("Comment %d", comment.ID))

		if resolveUnresolve {
			if err := client.UnresolveThread(comment.ThreadID); err != nil {
				fmt.Printf("%s Failed to unresolve %s: %v\n",
					ui.Colorize(ui.ColorRed, "❌"),
					ui.Colorize(ui.ColorCyan, commentLink),
					ui.Colorize(ui.ColorRed, err.Error()))
				errorCount++
			} else {
				fmt.Printf("%s %s marked as unresolved\n",
					ui.Colorize(ui.ColorYellow, "✓"),
					ui.Colorize(ui.ColorCyan, commentLink))
				successCount++
			}
		} else {
			if err := client.ResolveThread(comment.ThreadID); err != nil {
				fmt.Printf("%s Failed to resolve %s: %v\n",
					ui.Colorize(ui.ColorRed, "❌"),
					ui.Colorize(ui.ColorCyan, commentLink),
					ui.Colorize(ui.ColorRed, err.Error()))
				errorCount++
			} else {
				fmt.Printf("%s %s marked as resolved\n",
					ui.Colorize(ui.ColorGreen, "✓"),
					ui.Colorize(ui.ColorCyan, commentLink))
				successCount++
			}
		}
	}

	fmt.Printf("\n%s: %s, %s\n",
		ui.Colorize(ui.ColorCyan, "Summary"),
		ui.Colorize(ui.ColorGreen, fmt.Sprintf("%d successful", successCount)),
		ui.Colorize(ui.ColorRed, fmt.Sprintf("%d failed", errorCount)))
	return nil
}

func resolveIndividualComment(client *github.Client, prNumber int, commentID int64) error {
	// Fetch review comments to find the thread ID
	comments, err := client.FetchReviewComments(prNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch review comments: %w", err)
	}

	// Find the comment with the given ID
	var threadID string
	for _, comment := range comments {
		if comment.ID == commentID {
			threadID = comment.ThreadID
			break
		}
	}

	if threadID == "" {
		return fmt.Errorf("comment ID %d not found in PR #%d", commentID, prNumber)
	}

	// Resolve or unresolve the thread
	commentLink := ui.CreateHyperlink(fmt.Sprintf("https://github.com/%s/pull/%d#discussion_r%d", getRepoFromClient(client), prNumber, commentID),
		fmt.Sprintf("Comment %d", commentID))

	if resolveUnresolve {
		if err := client.UnresolveThread(threadID); err != nil {
			return fmt.Errorf("failed to unresolve thread: %w", err)
		}
		fmt.Printf("%s Thread for %s marked as unresolved\n",
			ui.Colorize(ui.ColorYellow, "✓"),
			ui.Colorize(ui.ColorCyan, commentLink))
	} else {
		if err := client.ResolveThread(threadID); err != nil {
			return fmt.Errorf("failed to resolve thread: %w", err)
		}
		fmt.Printf("%s Thread for %s marked as resolved\n",
			ui.Colorize(ui.ColorGreen, "✓"),
			ui.Colorize(ui.ColorCyan, commentLink))
	}

	return nil
}

// truncateString truncates a string to the specified length and adds "..." if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// getRepoFromClient extracts the repository name from the client
func getRepoFromClient(client *github.Client) string {
	// Use the global repoFlag if set
	if repoFlag != "" {
		return repoFlag
	}

	// Try to get repo from the client's internal method
	// We'll need to call the client's getRepo method
	repo, err := client.GetRepo()
	if err == nil && repo != "" {
		return repo
	}

	// Fallback - we'll construct the URL without the repo part
	return "owner/repo"
}
