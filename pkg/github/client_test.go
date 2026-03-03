package github

import "testing"

func TestExtractGitHubOwner(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{
			name:      "SSH format",
			remoteURL: "git@github.com:sideshowbarker/WebKit.git",
			want:      "sideshowbarker",
		},
		{
			name:      "SSH format with trailing newline",
			remoteURL: "git@github.com:user/repo.git\n",
			want:      "user",
		},
		{
			name:      "HTTPS format",
			remoteURL: "https://github.com/WebKit/WebKit.git",
			want:      "WebKit",
		},
		{
			name:      "HTTPS format without .git",
			remoteURL: "https://github.com/owner/repo",
			want:      "owner",
		},
		{
			name:      "non-GitHub URL",
			remoteURL: "git@gitlab.com:user/repo.git",
			want:      "",
		},
		{
			name:      "empty string",
			remoteURL: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGitHubOwner(tt.remoteURL)
			if got != tt.want {
				t.Errorf("extractGitHubOwner(%q) = %q, want %q", tt.remoteURL, got, tt.want)
			}
		})
	}
}
