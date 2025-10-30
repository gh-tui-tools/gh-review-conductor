package ai

import "context"

// AIProvider defines the interface for AI code assistance
type AIProvider interface {
	// ApplySuggestion takes a review suggestion and current file state,
	// returns an adapted patch that can be applied to the current file
	ApplySuggestion(ctx context.Context, req *SuggestionRequest) (*SuggestionResponse, error)

	// Name returns the provider name (e.g., "gemini", "openai", "claude")
	Name() string

	// Model returns the model name being used (e.g., "gemini-2.5-flash-lite-preview-09-2025", "gpt-4")
	Model() string
}

// SuggestionRequest contains all context needed for AI to apply a suggestion
type SuggestionRequest struct {
	// Review context
	ReviewComment    string // The reviewer's comment/explanation
	SuggestedCode    string // The suggested code from the review
	OriginalDiffHunk string // The diff hunk from when review was made
	CommentID        int64  // Comment ID for reference

	// Current file state
	FilePath           string // Path to the file
	CurrentFileContent string // Full current file content
	TargetLineNumber   int    // Approximate line where change should go (0-based)

	// Additional context
	ExpectedLines []string // Lines we expected to find (from diff hunk)
	FileLanguage  string   // Programming language (go, python, etc.)

	// Failure context (optional)
	MismatchDetails string // What went wrong with traditional application
}

// SuggestionResponse contains the AI-generated patch
type SuggestionResponse struct {
	// The generated unified diff patch ready for git apply
	Patch string

	// Explanation of what the AI did (shown to user)
	Explanation string

	// Confidence level (0.0-1.0) - could inform user decisions
	Confidence float64

	// Any warnings the AI identified
	Warnings []string
}
