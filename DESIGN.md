# Design Document: New functions and refinements for the “browse” command

This document describes the architecture and implementation of some added features and refinements to the `browse` command.

## Views

The `browse` command provides two interactive views:

- **List View** — Shows all review comments in a navigable list. Use arrow keys to select, Enter to view details.
- **Detail View** — Shows full information for a single comment: body, code context, replies, URL, and timestamp.

Most actions (Q, C, r/u, R/U, a, e, o) work in both views. Some are view-specific: `i` (refresh) only works in list view; `Ctrl+F`/`Ctrl+B` (page scroll) only in detail view.

## Features Overview

- **Quote Reply** — Reply to comments with the original quoted as a blockquote (`Q` and `C` keys)
- **Resolve/Unresolve** — Toggle comment resolution state with dynamic key bindings (`r`/`u` and `R`/`U` keys)
- **Detail View** — View more details; now includes the URL and timestamp for the review comment
- **Refresh** — Fetch latest comments from GitHub without restarting (`i` key)
- **Editor Actions** — Async editor integration for composing replies
- **Confirmation Dialog** — Success feedback after posting comments
- **Coding Agent** — Launch a coding agent with the review comment context (`a` key)
- **Edit File** — Open the commented file in your editor at the exact line (`e` key)
- **Emoji Reactions** — Add emoji reactions to review comments (`x` key)

---

## Quote Reply Feature

The quote reply feature allows users to reply to PR review comments with the original comment quoted as a markdown blockquote. Two variants are available:

- **Q key**: Quote the comment body only
- **C key**: Quote with code context (includes the diff hunk)

### Feature Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                  User in List View or Detail View                │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  User presses Q or C  │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │   Prepare quoted      │
                    │   content with        │
                    │   FormatQuotedReply() │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Create temp file     │
                    │  with quoted content  │
                    │  + instruction comment│
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Suspend TUI via      │
                    │  tea.ExecProcess()    │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Launch $EDITOR       │
                    │  with temp file       │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  User edits and       │
                    │  saves file           │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Editor exits,        │
                    │  TUI resumes          │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Read temp file,      │
                    │  sanitize content     │
                    │  (strip # lines)      │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  POST to GitHub API   │
                    │  via gh api -F        │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Show confirmation    │
                    │  dialog with URL      │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  User presses any     │
                    │  key to dismiss       │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Return to previous   │
                    │  view                 │
                    └───────────────────────┘
```

### Quote Format

#### Q key (quote only)

```markdown
> @author wrote:
>
> [original comment body]

[cursor here for user's reply]
```

#### C key (quote with context)

```markdown
> ```diff
> --- a/path/to/file.go
> +++ b/path/to/file.go
> @@ -10,5 +10,7 @@
>  context line
> +added line
> -removed line
> ```
>
> @author wrote:
>
> [original comment body]

[cursor here for user's reply]
```

The diff context uses git-style headers (`--- a/` and `+++ b/`) for familiarity.

---

## Resolve/Unresolve Feature

### Dynamic Key Bindings

The help text dynamically changes based on the selected comment's resolved state:

- When an **unresolved** comment is selected: `r resolve` and `R resolve+comment`
- When a **resolved** comment is selected: `u unresolve` and `U unresolve+comment`

Both `r` and `u` trigger the same toggle action, as do `R` and `U`. The dynamic help text guides users to the appropriate key for their intent.

### Implementation

The `isItemResolved` callback is provided to the selector, which uses it to determine which action key description to display:

```go
func (m *SelectionModel[T]) getResolveActionKey() string {
    if m.isSelectedResolved() && m.actionKeyAlt != "" {
        return m.actionKeyAlt
    }
    return m.actionKey
}
```

---

## Detail View Feature

The detail view shows full comment information including replies, code context, and suggested changes.

### Comment Metadata

The detail view header displays:

```
Author: @username
Location: path/to/file.go:42
Status: unresolved (or resolved)
URL: https://github.com/owner/repo/pull/123#discussion_r123456789
Time: 10 hours ago
```

- **URL**: Clickable hyperlink (OSC8) to the comment on GitHub
- **Time**: Human-readable relative timestamp using `FormatRelativeTime()`

### Reply Formatting

Each reply in the `--- Replies ---` section shows:

```
Reply 1 by @author | https://github.com/.../discussion_r123 | 13 minutes ago

[reply body rendered as markdown]
```

### Loading Indicator

When pressing Enter to view details, a "Loading..." message is displayed while fetching fresh data from the GitHub API. This uses a deferred message pattern:

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  User hits   │────▶│ loadingDetail│────▶│   View()     │
│    Enter     │     │   = true     │     │  "Loading..."│
└──────────────┘     └──────────────┘     └──────────────┘
                            │
                            │ loadDetailMsg{}
                            ▼
                     ┌──────────────┐     ┌──────────────┐
                     │  onSelect()  │────▶│ showDetail   │
                     │  API fetch   │     │   = true     │
                     └──────────────┘     └──────────────┘
```

### Sticky Footer

The detail view has a sticky footer showing available actions:

```
esc/q back • o open • r resolve • R resolve+comment • Q quote • C quote+context
```

This matches the main list view's help text placement. The viewport height is reduced by 1 line to reserve space for the footer, ensuring it remains visible while scrolling.

---

## Refresh Feature

Pressing `i` in the list view fetches fresh data from the GitHub API and updates the entire list. This is useful when new comments have been added by other users or when you want to see the latest state without restarting the browser.

### Architecture

```
┌──────────────┐     ┌──────────────┐     ┌───────────────┐
│  User hits   │────▶│  refreshing  │────▶│   View()      │
│     'i'      │     │   = true     │     │"Refreshing..."│
└──────────────┘     └──────────────┘     └───────────────┘
                            │
                            │ refreshItems() called async
                            ▼
                     ┌──────────────┐
                     │  GitHub API  │
                     │    fetch     │
                     └──────────────┘
                            │
                            │ refreshFinishedMsg{items, err}
                            ▼
                     ┌──────────────┐     ┌──────────────┐
                     │   Update()   │────▶│ items updated│
                     │              │     │ list rebuilt │
                     └──────────────┘     └──────────────┘
```

The refresh callback is provided to the selector at initialization time, allowing the generic `SelectionModel` to trigger domain-specific data fetching without coupling to the GitHub client.

---

## Coding Agent Feature

Pressing `a` launches a coding agent (such as Claude Code) with the review comment context. This allows you to quickly hand off a review comment to an AI assistant for implementation.

### Configuration

The agent command is configured via the `GH_PRREVIEW_AGENT` environment variable:

```bash
# Default: uses 'claude' (Claude Code CLI)
gh prreview browse 123

# Use a different agent
GH_PRREVIEW_AGENT=aider gh prreview browse 123

# Test prompt format without launching agent
GH_PRREVIEW_AGENT=echo gh prreview browse 123
```

### Prompt Format

The agent receives the file path, line number, and full comment body:

```
Review comment on <path>:<line>

<full comment body>
```

Example:

```
Review comment on cmd/browse.go:131

In the `onSelect` handler, an error from `client.FetchReviewComments` is
silently ignored. If the API call fails, the `if err == nil` block is
skipped, and the detail view is shown with potentially stale data...
```

### Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  User hits   │────▶│ agentAction  │────▶│ Format prompt│
│     'a'      │     │   called     │     │ with context │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                                                 │ "LAUNCH_AGENT:prompt"
                                                 ▼
                                          ┌──────────────┐
                                          │ launchAgent()│
                                          │ reads $ENV   │
                                          └──────────────┘
                                                 │
                                                 │ tea.ExecProcess()
                                                 ▼
                                          ┌──────────────┐
                                          │ Agent runs   │
                                          │ (TUI suspend)│
                                          └──────────────┘
                                                 │
                                                 │ agentFinishedMsg{}
                                                 ▼
                                          ┌──────────────┐
                                          │ TUI resumes  │
                                          └──────────────┘
```

The agent is launched via `tea.ExecProcess()`, which suspends the TUI while the agent runs. When the agent exits, the TUI resumes and the user can continue browsing comments.

---

## Thread Comment Selection

When a review comment has multiple replies (a "thread"), pressing `Q`, `C`, or `a` allows you to select which specific comment in the thread to operate on.

### Behavior

- **Single comment threads**: Action proceeds immediately (no change from previous behavior)
- **Multi-comment threads**: Enter "comment selection mode" to cycle through comments

### User Flow (Detail View)

```
┌──────────────────────────────────────────────────────────────────┐
│  User viewing detail view of a thread with 3 comments            │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  User presses Q/C/a   │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Main comment gets    │
                    │  visual highlight:    │
                    │  ▶▶▶ SELECTED ◀◀◀     │
                    └───────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Status bar shows:    │
                    │  [1/3] @author: ...   │
                    │  (Enter=select,       │
                    │   Q=next, Esc=cancel) │
                    └───────────────────────┘
                                │
            ┌───────────────────┼───────────────────┐
            │                   │                   │
            ▼                   ▼                   ▼
    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
    │ Press same   │    │ Press Enter  │    │ Press Esc    │
    │ key again    │    │              │    │              │
    │ (Q/C/a)      │    │              │    │              │
    └──────────────┘    └──────────────┘    └──────────────┘
            │                   │                   │
            ▼                   ▼                   ▼
    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
    │ Cycle to     │    │ Execute      │    │ Cancel and   │
    │ next comment │    │ action on    │    │ return to    │
    │ [2/3]...     │    │ selected     │    │ normal view  │
    └──────────────┘    └──────────────┘    └──────────────┘
```

### Visual Highlighting

In detail view, the selected comment is wrapped with magenta-colored markers:

```
▶▶▶ SELECTED COMMENT ◀◀◀
--- Comment ---
[comment body rendered with markdown]
▶▶▶ END SELECTED ◀◀◀
```

For thread replies:

```
▶▶▶ SELECTED REPLY ◀◀◀
Reply 2 by @author | https://... | 5 minutes ago
[reply body]
▶▶▶ END SELECTED ◀◀◀
```

### Implementation

The `ItemRenderer` interface includes methods for thread comment support:

```go
// ThreadCommentCount returns the number of comments in this item's thread
// (1 = main only, >1 = main + replies). Return 0 if not applicable.
ThreadCommentCount(item T) int

// ThreadCommentPreview returns a preview string for the comment at index
// (0 = main comment, 1+ = thread replies)
ThreadCommentPreview(item T, idx int) string

// PreviewWithHighlight returns detailed preview with a specific comment highlighted
PreviewWithHighlight(item T, highlightIdx int) string

// WithSelectedComment returns a copy of item with the selected comment index set
WithSelectedComment(item T, idx int) T
```

The `SelectionModel` tracks comment selection state:

```go
commentSelectMode     bool        // true when cycling through comments
commentSelectAction   string      // "Q", "C", or "a"
commentSelectIdx      int         // 0 = main, 1+ = thread replies
commentSelectItem     listItem[T] // the item being operated on
commentSelectInDetail bool        // true if started from detail view
```

---

## Edit File Feature

Pressing `e` opens the commented file in your editor at the exact line number where the review comment was made. This allows you to quickly jump to the code being discussed.

### Configuration

The editor is configured via the standard `$EDITOR` environment variable (falls back to `vim` if not set).

### Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  User hits   │────▶│  editAction  │────▶│ Format path  │
│     'e'      │     │   called     │     │  and line    │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                                                 │ "EDIT_FILE:path:line"
                                                 ▼
                                          ┌──────────────┐
                                          │editInEditor()│
                                          │ reads $EDITOR│
                                          └──────────────┘
                                                 │
                                                 │ tea.ExecProcess()
                                                 ▼
                                          ┌──────────────┐
                                          │ Editor opens │
                                          │ at +line     │
                                          └──────────────┘
                                                 │
                                                 │ editorFinishedMsg{}
                                                 ▼
                                          ┌──────────────┐
                                          │ TUI resumes  │
                                          └──────────────┘
```

The editor is launched with the `+line` convention (e.g., `vim +42 file.go`) to position the cursor at the comment's line number.

---

## Emoji Reactions Feature

Pressing `x` allows you to add emoji reactions to review comments. This provides a quick way to acknowledge comments without typing a full reply.

### Supported Emojis

GitHub supports 8 emoji reactions:

| Emoji | Name |
|-------|------|
| 👍 | +1 |
| 👎 | -1 |
| 😄 | laugh |
| 😕 | confused |
| ❤️ | heart |
| 🎉 | hooray |
| 🚀 | rocket |
| 👀 | eyes |

### User Flow

```
┌──────────────────────────────────────────────────────────────────┐
│  User in List View or Detail View                                 │
└──────────────────────────────────────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  User presses 'x'     │
                    └───────────────────────┘
                                │
                    ┌───────────┴───────────┐
                    │                       │
                    ▼                       ▼
        ┌───────────────────┐   ┌───────────────────┐
        │ Single comment    │   │ Multi-comment     │
        │ thread            │   │ thread            │
        └───────────────────┘   └───────────────────┘
                    │                       │
                    │                       ▼
                    │           ┌───────────────────┐
                    │           │ Enter comment     │
                    │           │ selection mode    │
                    │           │ (x=cycle, Enter)  │
                    │           └───────────────────┘
                    │                       │
                    └───────────┬───────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Enter reaction mode  │
                    │  Status bar shows:    │
                    │  React: [1/8] +1      │
                    │  (x=next, Enter=add,  │
                    │   Esc=cancel)         │
                    └───────────────────────┘
                                │
            ┌───────────────────┼───────────────────┐
            │                   │                   │
            ▼                   ▼                   ▼
    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
    │ Press 'x'    │    │ Press Enter  │    │ Press Esc    │
    │              │    │              │    │              │
    └──────────────┘    └──────────────┘    └──────────────┘
            │                   │                   │
            ▼                   ▼                   ▼
    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
    │ Cycle to     │    │ POST to      │    │ Cancel and   │
    │ next emoji   │    │ GitHub API   │    │ return to    │
    │ [2/8] -1...  │    │              │    │ normal view  │
    └──────────────┘    └──────────────┘    └──────────────┘
                                │
                                ▼
                        ┌──────────────┐
                        │ Show success │
                        │ "Added       │
                        │ reaction: +1"│
                        └──────────────┘
```

### GitHub API

Reactions are added via the GitHub REST API:

```
POST /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions
Content-Type: application/json

{"content": "+1"}
```

### Implementation

The reaction feature uses a modal state similar to comment selection mode:

```go
// Reaction mode state in SelectionModel
reactionMode      bool   // true when cycling through reactions
reactionIdx       int    // current emoji index (0-7)
reactionCommentID int64  // comment ID to react to
```

The `SelectorOptions` struct includes reaction callbacks:

```go
// Action: x (add reaction)
ReactionAction   func(T) (int64, error)                       // Returns comment ID
ReactionComplete func(commentID int64, emoji string) (string, error) // Applies reaction, returns confirmation message
ReactionKey      string                                       // e.g., "x react"
```

### Confirmation Dialog

After successfully adding a reaction, a confirmation dialog is shown with the message returned by `ReactionComplete` (typically including a link to the comment on GitHub). The user presses any key to dismiss the dialog and return to the browse view.

### Integration with Thread Selection

For comments with replies, pressing `x` first enters comment selection mode (same as Q/C/a), allowing you to choose which specific comment to react to. After selecting, reaction mode begins.

---

## Editor Action Architecture

### Problem

The previous approach to editor-based actions (like "resolve with comment") used synchronous `exec.Command().Run()` which didn't properly handle the bubbletea TUI lifecycle. This could cause display issues when returning from the editor.

### Solution

We introduced an async editor action pattern using bubbletea's `tea.ExecProcess()`:

```
┌─────────────────────────────────────────────────────────────────┐
│                      SelectionModel[T]                          │
├─────────────────────────────────────────────────────────────────┤
│  Fields:                                                        │
│  - editorPrepareSecond/Third/Fourth: EditorPreparer[T]          │
│  - editorCompleteSecond/Third/Fourth: EditorCompleter[T]        │
│  - pendingEditorItem: T                                         │
│  - pendingEditorTmpFile: string                                 │
│  - pendingEditorAction: int (2=R, 3=Q, 4=C)                     │
│  - confirmationMessage: string                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Type Definitions

```go
// EditorPreparer returns initial content for the editor
type EditorPreparer[T any] func(item T) (string, error)

// EditorCompleter processes the edited content and returns a status message
type EditorCompleter[T any] func(item T, editorContent string) (string, error)

// editorFinishedMsg signals that the editor process has exited
type editorFinishedMsg struct {
    err error
}
```

### Message Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Update()   │────▶│ startEditor  │────▶│ tea.Exec     │
│              │     │ ForAction()  │     │ Process()    │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                     ┌───────────────────────────┘
                     │ Editor runs (TUI suspended)
                     ▼
              ┌──────────────┐
              │ Editor exits │
              └──────────────┘
                     │
                     │ editorFinishedMsg{err}
                     ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Update()   │────▶│ handleEditor │────▶│ completer()  │
│              │     │ Finished()   │     │              │
└──────────────┘     └──────────────┘     └──────────────┘
                                                 │
                                                 ▼
                                          ┌──────────────┐
                                          │ confirmation │
                                          │ Message set  │
                                          └──────────────┘
```

### Key Methods

#### `startEditorForAction(item T, actionNum int, initialContent string) tea.Cmd`

1. Creates a temp file with `.md` extension
2. Writes initial content + instruction template
3. Stores pending state (item, file path, action number)
4. Returns `tea.ExecProcess()` command that:
   - Suspends the TUI
   - Runs `$EDITOR` (or `vim` as fallback)
   - Sends `editorFinishedMsg` when editor exits

#### `handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd)`

1. Reads the temp file content
2. Sanitizes content (strips lines starting with `#`)
3. Validates content is not empty
4. Calls the appropriate completer based on `pendingEditorAction`
5. Sets `confirmationMessage` if completer returns a status
6. Cleans up temp file

---

## Confirmation Dialog

After successfully posting a comment, a confirmation dialog is displayed that persists until the user dismisses it:

```
╭─────────────────────────────────────────╮
│  ✓ Success                              │
│                                         │
│  Posted a comment to:                   │
│  https://github.com/.../pull/738#...    │
│                                         │
│  Press any key to continue...           │
╰─────────────────────────────────────────╯
```

### Implementation

- `confirmationMessage` field stores the message
- `renderConfirmation()` renders a centered, bordered box
- In `Update()`, any `tea.KeyMsg` while `confirmationMessage != ""` clears it

---

## Data Freshness

Two mechanisms ensure the view shows current data:

1. **Optimistic Updates**: After posting a reply via Q, C, or R, the new reply is immediately appended to the local `ThreadComments` slice. This provides instant feedback without waiting for an API refresh.

2. **Manual Refresh**: Pressing `i` in the list view fetches fresh data from the GitHub API and updates the entire list.

Note: The initial fetch retrieves all comment data including thread replies. Navigation and viewing use this cached data for instant response. Use `i` to refresh if you need to see changes made by others.

---

## Performance Optimizations

### Cached Comment Data

All comment data is fetched once at startup and cached in memory. Subsequent operations use this cached data:

- Viewing detail view uses cached data (no API call)
- Opening comments in browser uses cached URLs
- Thread comment selection uses cached thread data
- Only mutations (resolve, post reply) and explicit refresh (`i`) make API calls

### Cached Markdown Renderer

The glamour markdown renderer is expensive to create. A single renderer instance is created on first use and reused for all subsequent markdown rendering:

```go
var cachedMarkdownRenderer *glamour.TermRenderer
var rendererInitOnce sync.Once

func getMarkdownRenderer() *glamour.TermRenderer {
    rendererInitOnce.Do(func() {
        cachedMarkdownRenderer = glamour.NewTermRenderer(
            glamour.WithStandardStyle("dark"), // Avoid slow terminal detection
            glamour.WithWordWrap(80),
        )
    })
    return cachedMarkdownRenderer
}
```

This eliminates lag when cycling through thread comments, as each cycle previously created multiple new renderers.

### Markdown Warmup

The first call to the markdown renderer can be slow due to chroma lexer initialization. To avoid this delay when the user first views a comment detail, we warm up the renderer in the background at startup:

```go
func WarmupMarkdownRenderer() {
    go func() {
        r := getMarkdownRenderer()
        if r != nil {
            // Trigger lexer initialization with common code blocks
            r.Render("```go\nfunc main() {}\n```")
            r.Render("```js\nconst x = 1;\n```")
        }
    }()
}
```

This runs in a background goroutine while the GitHub API fetches comments, so by the time the user presses Enter to view details, the renderer is already warmed up.

### Pre-compiled Regexes

Regular expressions are compiled once at package init time rather than on each function call:

```go
// Package-level pre-compiled regexes
var (
    suggestionBlockRe = regexp.MustCompile("(?s)```suggestion\\s*\\n.*?```")
    imageMarkdownRe   = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
    diffHeaderRe      = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)
)
```

This avoids regex recompilation overhead on each call to functions like `StripSuggestionBlock()` and `ParseDiffHunk()`.

---

## Key Bindings

| Key | Context | Action |
|-----|---------|--------|
| `q` | List view | Quit application |
| `q` | Detail view | Go back to list view |
| `esc` | Detail view | Go back to list view |
| `Q` | Both | Quote reply (no context) |
| `C` | Both | Quote reply with code context |
| `r` | Both | Resolve (shown when comment is unresolved) |
| `u` | Both | Unresolve (shown when comment is resolved) |
| `R` | Both | Resolve + comment (shown when unresolved) |
| `U` | Both | Unresolve + comment (shown when resolved) |
| `a` | Both | Launch coding agent with comment context |
| `e` | Both | Open file in editor at comment line |
| `x` | Both | Add emoji reaction to comment |
| `i` | List view | Refresh (fetch new comments from GitHub) |
| `o` | Both | Open in browser |
| `enter` | List view | Show detail view |
| `Ctrl+F` | Detail view | Page down (scroll one viewport forward) |
| `Ctrl+B` | Detail view | Page up (scroll one viewport back) |

Note: Key bindings are case-sensitive. The `q` key behaves differently in list vs detail view for convenience.

---

## Selector API (SelectorOptions)

The interactive selector uses an options struct pattern for clean, readable configuration:

```go
func Select[T any](opts SelectorOptions[T]) (T, error)
```

### SelectorOptions Structure

```go
type SelectorOptions[T any] struct {
    // Required
    Items    []T
    Renderer ItemRenderer[T]

    // Core callbacks
    OnSelect       CustomAction[T]        // Called when Enter is pressed
    OnOpen         CustomAction[T]        // Called when 'o' is pressed
    FilterFunc     func(T, bool) bool     // Filter items based on state
    IsItemResolved func(T) bool           // For dynamic key display (r vs u)
    RefreshItems   func() ([]T, error)    // Called when 'i' is pressed

    // Action: r/u (resolve toggle)
    ResolveAction CustomAction[T]
    ResolveKey    string // e.g., "r resolve"
    ResolveKeyAlt string // e.g., "u unresolve"

    // Action: R/U (resolve+comment via editor)
    ResolveCommentPrepare  EditorPreparer[T]
    ResolveCommentComplete EditorCompleter[T]
    ResolveCommentKey      string // e.g., "R resolve+comment"
    ResolveCommentKeyAlt   string // e.g., "U unresolve+comment"

    // Action: Q (quote reply via editor)
    QuotePrepare  EditorPreparer[T]
    QuoteComplete EditorCompleter[T]
    QuoteKey      string // e.g., "Q quote"

    // Action: C (quote+context via editor)
    QuoteContextPrepare  EditorPreparer[T]
    QuoteContextComplete EditorCompleter[T]
    QuoteContextKey      string // e.g., "C quote+context"

    // Action: a (launch agent)
    AgentAction CustomAction[T]
    AgentKey    string // e.g., "a agent"

    // Action: e (edit file)
    EditAction CustomAction[T]
    EditKey    string // e.g., "e edit"

    // Action: x (add reaction)
    ReactionAction   func(T) (int64, error)                       // Returns comment ID
    ReactionComplete func(commentID int64, emoji string) (string, error) // Applies reaction, returns confirmation message
    ReactionKey      string                                       // e.g., "x react"
}
```

### Design Rationale

The options struct pattern replaced a previous function with 29+ positional parameters. Benefits:

1. **Readability**: Named fields make call sites self-documenting
2. **Maintainability**: Adding new options doesn't break existing callers
3. **Optional fields**: Zero values disable features (no need for `nil` placeholders)
4. **Grouped logic**: Related options (e.g., action + key) are visually adjacent

### Usage Example

```go
selected, err := ui.Select(ui.SelectorOptions[BrowseItem]{
    Items:    browseItems,
    Renderer: renderer,
    OnSelect: onSelect,
    OnOpen:   openAction,

    ResolveAction: resolveAction,
    ResolveKey:    "r resolve",
    ResolveKeyAlt: "u unresolve",

    QuotePrepare:  editorPrepareQ,
    QuoteComplete: editorCompleteQ,
    QuoteKey:      "Q quote",

    AgentAction: agentAction,
    AgentKey:    "a agent",

    ReactionAction:   reactionAction,
    ReactionComplete: reactionComplete,
    ReactionKey:      "x react",
})
```

### Editor Actions

When an `EditorPreparer` is provided for an action, pressing the key opens `$EDITOR`:

1. `EditorPreparer(item)` returns initial content (or error to abort)
2. User edits content in their editor
3. `EditorCompleter(item, editedContent)` processes the result

The `SanitizeEditorContent()` helper strips trailing `# comment` lines from editor output.

---

## Shared Utility Functions

### Diff Processing

| Function | Purpose |
|----------|---------|
| `ColorizeDiff(diff)` | Apply terminal colors to diff lines (+green, -red, @cyan) |
| `TruncateDiff(diff, maxLines)` | Limit diff output with "..." suffix |
| `FormatDiffWithHeaders(hunk, path)` | Add git-style `--- a/` and `+++ b/` headers |

The detail view uses `TruncateDiff` + `ColorizeDiff` for the context preview. The quote-with-context feature uses `FormatDiffWithHeaders` for the markdown output.

### Time Formatting

| Function | Purpose |
|----------|---------|
| `FormatRelativeTime(t time.Time)` | Format timestamps as human-readable relative strings |

`FormatRelativeTime` returns strings like:
- "just now" (< 1 minute)
- "5 minutes ago"
- "2 hours ago"
- "3 days ago"
- "1 month ago"
- "2 years ago"

This matches the style used by the GitHub web UI for comment timestamps.

---

## Files Changed

| File | Changes |
|------|---------|
| `pkg/ui/quote.go` | **New** - `FormatQuotedReply()`, `FormatBlockquote()` |
| `pkg/ui/colors.go` | Added `FormatDiffWithHeaders()`, `TruncateDiff()`, `FormatRelativeTime()`, `ColorMagenta`, cached markdown renderer (`cachedMarkdownRenderer`, `getMarkdownRenderer()`) |
| `pkg/ui/selector.go` | **Refactored to `SelectorOptions[T]` struct** replacing 29+ positional parameters with named fields. Editor action system, confirmation dialog, Q/C handlers, loading indicator, sticky footer, `loadDetailMsg` type, dynamic resolve/unresolve keys (`u`/`U`), `isItemResolved` callback, `refreshItems` callback, `i` key refresh handler, `a` key agent launcher, `launchAgent()` method, `agentFinishedMsg` type, `e` key edit file handler, thread comment selection state (`commentSelectMode`, `commentSelectIdx`, etc.), `PreviewWithHighlight` interface method, reaction mode state (`reactionMode`, `reactionIdx`, `reactionCommentID`), `ReactionAction`/`ReactionComplete`/`ReactionKey` options |
| `pkg/ui/selector_nocov.go` | Added `reactionEmojis` constant, `x` key handler for reaction mode, `enterReactionMode()` and `showReactionStatus()` helper methods, reaction mode handling in Update loop |
| `cmd/browse.go` | Editor prepare/complete functions, optimistic reply updates, URL/timestamp display in details, refresh callback for `i` key, `agentAction` for launching coding agent, `editAction` for opening file in editor, `SelectedCommentIdx` field in `BrowseItem`, `openURLInBrowser()` helper, removed redundant API calls for caching, `reactionAction` and `reactionComplete` callbacks for emoji reactions |
| `cmd/comment.go` | Uses shared `SanitizeEditorContent()` |
| `pkg/github/client.go` | Fixed `-f` to `-F` for file reading in `gh api`, added `CreatedAt` field to `ReviewComment` and `ThreadComment` structs, added `AddReactionToComment()` method for adding emoji reactions to review comments |

---

## Bug Fix: gh api -f vs -F

The `gh api` command uses different flags for form fields:

- `-f key=value` - Raw string field (literal value)
- `-F key=value` - Typed field (supports `@file` for file reading)

The `@file` syntax for reading file contents only works with `-F`. Using `-f body=@/path/to/file` would post the literal string `@/path/to/file` instead of the file contents.
