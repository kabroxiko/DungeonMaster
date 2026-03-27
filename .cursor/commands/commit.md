---
description: Interactive `/commit` assistant command: propose commit message from git diff, stage all changes, and commit after confirmation.
alwaysApply: true
---

# /commit — propose, refine, and run a git commit

When the user invokes the `/commit` command, the assistant will:

1. Inspect the repository and working tree:
   - Run `git status --porcelain --branch` to get branch status and changed files.
   - Run `git diff --name-status HEAD` to list changed file paths and operations.
   - Optionally show the unified diff for specific files on request.
2. Run quick safety checks:
   - Detect obvious secrets or sensitive files (e.g. `.env`, credentials) and warn the user instead of committing them automatically.
   - Run project lints for recently edited files (use ReadLints) and summarize any new linter errors that would be committed.
3. Synthesize a detailed, human-friendly commit message:
   - Produce a one-line header using conventional prefixes (e.g. `feat:`, `fix:`, `chore:`) plus a concise scope and short description.
   - Provide a multi-line body (2–8 lines) explaining the rationale and high-level changes — focus on why the change was made and any important notes for reviewers.
   - When appropriate, include a brief test plan or mention of follow-up tasks.
4. Present the proposed commit message and summary to the user and ask for action:
   - Confirm: commit with the suggested message.
   - Edit: provide an edited message to use instead.
   - Inspect: request diffs or file lists before deciding.
   - Cancel: abort without making changes.
5. On user approval:
   - Stage files (`git add -A`) and create the commit (`git commit -m "<final message>"`).
   - Report the new commit short hash and updated branch status (e.g., ahead/behind origin).

Rules & constraints:
- Never push to any remote automatically. Pushing requires explicit user permission.
- If secrets or files flagged by the project's `.gitignore` are staged, warn and require explicit confirmation before committing.
- If linter errors are present in the staged changes, summarize them and ask whether to proceed.
- Keep commit messages focused on the "why" and include minimal necessary "what". Prefer small, focused commits.

Example flows:
- Quick commit:
  - User: `/commit`
  - Assistant: shows status/diff, proposes a one-line header + short body, user replies "Yes" → assistant commits.
- Edit message:
  - User: `/commit`
  - Assistant: proposes message, user replies "Edit: fix: normalize newline handling in parser\n\nFixes inconsistent CRLF behavior across platforms." → assistant commits with the edited message.

Implementation notes for the assistant:
- Use `ReadLints` to check recent edits for linter issues before committing.
- If the repository contains very large files (>5MB) in the staged set, warn the user and request confirmation.
- Always display the commit hash and a concise summary of changed files after committing.

