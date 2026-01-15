# Design Document: Selector API

This document describes the architecture of the interactive selector component.

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
})
```

### Editor Actions

When an `EditorPreparer` is provided for an action, pressing the key opens `$EDITOR`:

1. `EditorPreparer(item)` returns initial content (or error to abort)
2. User edits content in their editor
3. `EditorCompleter(item, editedContent)` processes the result

The `SanitizeEditorContent()` helper strips trailing `# comment` lines from editor output.
