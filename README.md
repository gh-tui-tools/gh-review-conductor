# gh-prreview

A GitHub CLI extension that applies review comments and suggestions directly to
your local checkout.

<img width="1752" height="1073" alt="image" src="https://github.com/user-attachments/assets/544dfab5-1519-415a-866d-8e85b796ae76" />

## Overview

Use `gh prreview` to pull review comments, preview suggested changes, and apply
them interactively or in bulk, all from the terminal.

## Installation

```bash
gh extension install chmouel/gh-prreview
```

Or build from source:

```bash
git clone https://github.com/chmouel/gh-prreview
cd gh-prreview
go build
gh extension install .
```

## Commands

### List

Fetch unresolved comments for the current PR (or pass `[PR_NUMBER] [THREAD_ID]`).

```bash
gh prreview list [PR_NUMBER] [THREAD_ID]
gh prreview list --all
gh prreview list --json
```

### Apply

Preview and apply suggestions interactively, or add `--all`, `--file`, or
`--include-resolved` for batch updates. `--debug` prints verbose logs and AI flags
(--ai-auto, --ai-provider, --ai-model, --ai-template, --ai-token) help with
conflicting cases.

```bash
gh prreview apply [PR_NUMBER]
gh prreview apply --all [PR_NUMBER]
```

**Tip:** keep a clean working tree before running apply.

### Browse

Navigate review comments in an interactive selector, jump to a specific comment,
or open it in your browser.

```bash
gh prreview browse
gh prreview browse <COMMENT_ID>
```

### Resolve

Resolve or unresolve threads, add comments, or resolve all for the current PR.

```bash
gh prreview resolve [COMMENT_ID]
gh prreview resolve --all
```

### Comment

Reply via editor, inline `--body`, file, or stdin input. Use `--resolve` to mark
threads resolved after replying.

```bash
gh prreview comment <COMMENT_ID> [PR_NUMBER]
```

## Features

- fetches GitHub review comments and parses suggestion blocks
- previews and applies suggestions with an interactive UI
- browse comments, resolve threads, and reply without leaving the terminal
- optional AI-assisted application for fuzzy or outdated suggestion hunks

## Requirements

- GitHub CLI (`gh`) installed and authenticated
- Git repository with a GitHub remote and an active PR

## License

[Apache License 2.0](./LICENSE)
