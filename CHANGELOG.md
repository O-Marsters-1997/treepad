# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`tp ui`** — New interactive fleet view command for monitoring and managing all worktrees from one screen. Features live-updating terminal UI with keyboard navigation, direct worktree actions (sync, remove, prune, open), and cd-on-exit support.

### Removed
- **`tp status --watch`** — Replaced by the new `tp ui` command for a more feature-rich, interactive fleet management experience. Use `tp ui` for live fleet monitoring instead.
