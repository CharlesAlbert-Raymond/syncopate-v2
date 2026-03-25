You are reviewing the synco codebase. Perform a thorough audit of **all changed files** compared to the main branch. Run all checks in parallel where possible.

## Steps

### 1. Identify what changed
Run `git diff main...HEAD --name-only` to get the list of changed files. Read each changed file fully. Also read the files they interact with (imports, callers) to understand impact.

### 2. Best practices audit
For each changed file, check:
- **Error handling**: Are errors properly propagated? No silently swallowed errors? Descriptive error messages?
- **Security**: Any command injection risks (especially in tmux/git exec calls)? User input sanitized?
- **Naming**: Do function/variable names follow Go conventions and match the existing codebase style?
- **API design**: Are public function signatures clean? Would a caller find them intuitive?
- **Edge cases**: What happens with empty strings, nil values, missing sessions, non-existent branches?

### 3. Code duplication
Search the codebase for duplicated logic:
- Compare the new code against existing code in the TUI (create.go, confirm.go, list.go, app.go)
- Flag any sequences that are copy-pasted and should be extracted into shared helpers
- Check if any new utility functions duplicate existing ones

### 4. Test coverage
- Check if tests exist for the changed packages (`*_test.go` files)
- List which functions have tests and which don't
- Identify the highest-value test cases that are missing (focus on logic-heavy functions, not simple wrappers)
- Don't write tests — just report what's missing

### 5. MCP parity
This is a critical check. Compare what the TUI can do vs what the MCP server exposes:
- List all user-facing TUI actions (from app.go, list.go, create.go, confirm.go, sidebar interactions)
- List all MCP tools (from internal/mcp/tools.go)
- Flag any TUI capability that has NO corresponding MCP tool
- Flag any MCP tool that behaves differently from its TUI counterpart (different defaults, missing steps, etc.)

### 6. Architecture review
- Is the package structure clean? Any circular or unnecessary dependencies?
- Is the MCP server properly decoupled from the TUI?
- Could any new code break existing functionality?

## Output format

Present your findings as a structured report:

```
## Review: [branch name]

### Summary
[1-2 sentence overall assessment]

### Best Practices
- ✅ [thing that's good]
- ⚠️ [concern] — [file:line] — [suggestion]
- ❌ [issue] — [file:line] — [what to fix]

### Code Duplication
- [description of duplicated logic and where to extract it]

### Test Coverage
| Package | Files | Has Tests | Missing Coverage |
|---------|-------|-----------|-----------------|
| ...     | ...   | yes/no    | ...             |

### MCP Parity
| TUI Action | MCP Tool | Status |
|-----------|----------|--------|
| ...       | ...      | ✅/❌/⚠️ |

### Architecture
- [findings]

### Recommended Actions (priority ordered)
1. [most important fix]
2. ...
```
