# Instructions for AI Agents

This file is for AI coding agents (Kiro, Copilot, Claude, Codex, Cursor, and similar tools) working in this repository. Follow these rules in every session.

## Use the official GitHub MCP server

**Always use the official GitHub MCP server ([github/github-mcp-server](https://github.com/github/github-mcp-server)) for all git and GitHub operations.**

- Use GitHub MCP tools for commits, pushes, file updates, issues, pull requests, and repository queries.
- Do not use the `git` or `gh` command line tools, even when this file or other documents mention a command line instruction. Treat any command named here as a description of the operation, and perform it with the equivalent GitHub MCP tool.
- If the GitHub MCP server is unavailable or lacks a tool for the operation, stop and ask the user. Do not fall back to the command line.

## Branch policy

**Always work on the `main` branch. Never create any other branch.**

- Make all commits directly on `main`.
- Never call the GitHub MCP `create_branch` tool, and never pass a branch other than `main` to any tool that accepts a branch parameter.
- Do not create feature, fix, topic, or temporary branches, even if a tool, template, or workflow suggests it. This includes the equivalents of `git branch <name>`, `git checkout -b`, `git switch -c`, and `git worktree add`.
- Do not open pull requests from new branches.
- Before making changes, confirm that you are targeting `main`. If anything points at another branch, stop and ask the user.
- If a task seems to require a separate branch, stop and ask the user instead of creating one.

## Other rules

- Do not force push or rewrite history on `main`.
- Do not perform destructive operations, such as the equivalents of `git reset --hard` or `git clean -fd`, unless the user asks for it.
- Keep commits small and give them clear messages.

## Project documents

- `requirements.md`: project requirements
- `architecture.md`: architecture and design
- `review.md`: review notes
