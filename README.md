# treepad

A CLI for managing git worktrees — providing a standardised, extensible set of utilities for working with worktrees.

## Overview

`treepad` makes it easy to create, navigate, and manage git worktrees from the command line. The aim is to build a consistent, composable toolset around worktree workflows that can be extended as new patterns emerge.

A primary motivation is **parallelising [Claude Code](https://claude.ai/code) instances**: each worktree gets its own isolated working directory, allowing multiple AI coding sessions to run simultaneously on different tasks without interfering with each other. `treepad` provides the primitives to spin up, coordinate, and share context between those worktrees.

## Use cases

- Spin up a worktree per task or feature branch and keep them organised
- Parallelise Claude Code instances across worktrees for concurrent AI-assisted development
- Share context (notes, plans, scratch files) between worktrees in a structured way
- Standardise worktree lifecycle management across a team or project

## Built with

- [urfave/cli v3](https://cli.urfave.org/) — composable, extensible CLI framework for Go
  - Details for v3 can be found at https://cli.urfave.org/v3/getting-started/

## Installation

```bash
go install treepad@latest
```

> Installation instructions will be updated as the project matures.

## Usage

```
treepad [command] [options]
```

See [docs/commands.md](docs/commands.md) for the full command reference.
