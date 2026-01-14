package ui

import (
	"errors"
	"testing"
)

func TestSanitizeEditorContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "no comment lines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "only comment lines",
			input:    "# Comment 1\n# Comment 2",
			expected: "",
		},
		{
			name:     "mixed content with # in middle preserved",
			input:    "User content\n# This is a comment\nMore content",
			expected: "User content\n# This is a comment\nMore content",
		},
		{
			name:     "# at start preserved",
			input:    "# Header comment\nActual content",
			expected: "# Header comment\nActual content",
		},
		{
			name:     "comment at end",
			input:    "Content here\n# Footer comment",
			expected: "Content here",
		},
		{
			name:     "whitespace trimming",
			input:    "  \n\nContent\n\n  ",
			expected: "Content",
		},
		{
			name:     "preserves internal whitespace",
			input:    "Line 1\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "typical editor template",
			input:    "> @author wrote:\n>\n> Original comment\n\nMy reply here\n# Write your comment above. Lines starting with # are ignored.\n",
			expected: "> @author wrote:\n>\n> Original comment\n\nMy reply here",
		},
		{
			name:     "hash in middle of line preserved",
			input:    "Code with # comment",
			expected: "Code with # comment",
		},
		{
			name:     "markdown heading preserved",
			input:    "# Heading\nContent",
			expected: "# Heading\nContent",
		},
		{
			name:     "markdown heading with trailing template",
			input:    "# Heading\nContent\n# Template comment",
			expected: "# Heading\nContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeEditorContent(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeEditorContent(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestEditorPrepareComplete tests the type signatures compile correctly
func TestEditorPreparerType(t *testing.T) {
	// This test verifies the EditorPreparer type works as expected
	preparer := EditorPreparer[string](func(item string) (string, error) {
		return "prepared: " + item, nil
	})

	result, err := preparer("test")
	if err != nil {
		t.Errorf("EditorPreparer returned unexpected error: %v", err)
	}
	if result != "prepared: test" {
		t.Errorf("EditorPreparer result = %q, want %q", result, "prepared: test")
	}
}

func TestEditorCompleterType(t *testing.T) {
	// This test verifies the EditorCompleter type works as expected
	completer := EditorCompleter[string](func(item string, content string) (string, error) {
		return "completed: " + item + " with " + content, nil
	})

	result, err := completer("item", "content")
	if err != nil {
		t.Errorf("EditorCompleter returned unexpected error: %v", err)
	}
	if result != "completed: item with content" {
		t.Errorf("EditorCompleter result = %q, want %q", result, "completed: item with content")
	}
}

func TestCustomActionType(t *testing.T) {
	// This test verifies the CustomAction type works as expected
	action := CustomAction[int](func(item int) (string, error) {
		if item > 0 {
			return "positive", nil
		}
		return "non-positive", nil
	})

	result, err := action(5)
	if err != nil {
		t.Errorf("CustomAction returned unexpected error: %v", err)
	}
	if result != "positive" {
		t.Errorf("CustomAction result = %q, want %q", result, "positive")
	}

	result, err = action(-1)
	if err != nil {
		t.Errorf("CustomAction returned unexpected error: %v", err)
	}
	if result != "non-positive" {
		t.Errorf("CustomAction result = %q, want %q", result, "non-positive")
	}
}

func TestAgentFinishedMsgType(t *testing.T) {
	// This test verifies the agentFinishedMsg type works as expected
	msg := agentFinishedMsg{err: nil}
	if msg.err != nil {
		t.Errorf("agentFinishedMsg with nil error should have nil err")
	}

	expectedErr := "test error"
	msg = agentFinishedMsg{err: errors.New(expectedErr)}
	if msg.err == nil {
		t.Errorf("agentFinishedMsg with error should have non-nil err")
	}
	if msg.err.Error() != expectedErr {
		t.Errorf("agentFinishedMsg error = %q, want %q", msg.err.Error(), expectedErr)
	}
}

func TestLaunchAgentPrefix(t *testing.T) {
	// Test that LAUNCH_AGENT: prefix is correctly parsed
	tests := []struct {
		name           string
		input          string
		shouldLaunch   bool
		expectedPrompt string
	}{
		{
			name:           "valid launch agent prefix",
			input:          "LAUNCH_AGENT:Review comment on file.go:42\n\nComment body",
			shouldLaunch:   true,
			expectedPrompt: "Review comment on file.go:42\n\nComment body",
		},
		{
			name:           "no prefix",
			input:          "Some other result",
			shouldLaunch:   false,
			expectedPrompt: "",
		},
		{
			name:           "empty prompt after prefix",
			input:          "LAUNCH_AGENT:",
			shouldLaunch:   true,
			expectedPrompt: "",
		},
		{
			name:           "similar but not matching prefix",
			input:          "LAUNCH_AGENT_OTHER:something",
			shouldLaunch:   false,
			expectedPrompt: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := "LAUNCH_AGENT:"
			hasPrefix := len(tt.input) >= len(prefix) && tt.input[:len(prefix)] == prefix
			if hasPrefix != tt.shouldLaunch {
				t.Errorf("HasPrefix(%q, %q) = %v, want %v", tt.input, prefix, hasPrefix, tt.shouldLaunch)
			}
			if hasPrefix {
				prompt := tt.input[len(prefix):]
				if prompt != tt.expectedPrompt {
					t.Errorf("Prompt = %q, want %q", prompt, tt.expectedPrompt)
				}
			}
		})
	}
}

func TestSplitActionKey(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedKey  string
		expectedDesc string
	}{
		{
			name:         "simple action key",
			input:        "r resolve",
			expectedKey:  "r",
			expectedDesc: "resolve",
		},
		{
			name:         "uppercase key",
			input:        "R resolve+comment",
			expectedKey:  "R",
			expectedDesc: "resolve+comment",
		},
		{
			name:         "multi-word description",
			input:        "Q quote reply",
			expectedKey:  "Q",
			expectedDesc: "quote reply",
		},
		{
			name:         "unresolve key",
			input:        "u unresolve",
			expectedKey:  "u",
			expectedDesc: "unresolve",
		},
		{
			name:         "uppercase unresolve key",
			input:        "U unresolve+comment",
			expectedKey:  "U",
			expectedDesc: "unresolve+comment",
		},
		{
			name:         "no description",
			input:        "x",
			expectedKey:  "x",
			expectedDesc: "",
		},
		{
			name:         "empty string",
			input:        "",
			expectedKey:  "",
			expectedDesc: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, desc := splitActionKey(tt.input)
			if key != tt.expectedKey {
				t.Errorf("splitActionKey(%q) key = %q, want %q", tt.input, key, tt.expectedKey)
			}
			if desc != tt.expectedDesc {
				t.Errorf("splitActionKey(%q) desc = %q, want %q", tt.input, desc, tt.expectedDesc)
			}
		})
	}
}
