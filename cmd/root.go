package cmd

import (
	"github.com/spf13/cobra"
)

var repoFlag string

var rootCmd = &cobra.Command{
	Use:   "gh-prreview",
	Short: "Apply GitHub review comments directly to your code",
	Long: `gh-prreview is a GitHub CLI extension that allows you to fetch and apply
review comments and suggestions from pull requests directly to your local code.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return cmd.Help()
		}
		return browseCmd.RunE(browseCmd, []string{})
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&repoFlag, "repo", "R", "", "Select a repository using the OWNER/REPO format")
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(resolveCmd)
	rootCmd.AddCommand(commentCmd)
	rootCmd.AddCommand(browseCmd)
}
