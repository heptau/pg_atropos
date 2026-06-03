# pg_atropos Development Guide

## Project Overview

Go utility that splits PostgreSQL custom-format (`pg_dump -Fc`) dumps into individual files organized for GIT versioning. Uses `pg_restore -f -` pipe for reliable parsing (no text dump regex issues). Supports two modes: `origin` (preserves dump structure) and `custom` (lowercase directories).

## Architecture

- `main.go` - entry point (standalone, no external deps, stdlib `flag` package)
- `pipes pg_restore -f -` for SQL extraction (streaming, no temp SQL files)
- Stdin pipe support via `-f -` (pg_restore reads dump from stdin directly)

## Building

```bash
make build      # Build binary for current platform
make all        # Build binaries for all platforms
make test       # Run tests
make coverage   # Run tests with coverage
make lint       # Run linters (golangci-lint)
make clean      # Remove built artifacts
make release    # Version prompt → build all → gh release → homebrew formula
```

## Releasing

`make release` does all of the following:

1. **Prompts** for new version (or keeps current)
2. **Updates** `VERSION` + `main.go` version string, commits and tags (`v<ver>`)
3. **Builds** binaries for all 4 platforms (`make build-all`)
4. **Archives** each binary as `dist/pg_atropos-<ver>-<os>-<arch>.tar.gz`
5. **Publishes** GitHub release via `gh release create`
6. **Generates** Homebrew formula in `../homebrew-tap/Formula/pg_atropos.rb`
7. **Pushes** the formula to `heptau/tap`

**Prerequisite:** `gh` CLI (`brew install gh`)

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | `""` | Database name to dump |
| `--conn` | `""` | PostgreSQL connection string |
| `--file` | `""` | Custom-format dump file (`"-"` for stdin) |
| `--output` | `./output` | Output directory |
| `--mode` | `origin` | Output mode: origin\|custom |
| `--clean` | `false` | Clean output directory before processing |
| `--no-db-path` | `false` | Don't include database name in output path |
| `--blacklist-db` | `^(template\|postgres)` | Skip databases matching pattern |
| `--whitelist-db` | `""` | Only include databases matching pattern |
| `--exclude-obj` | `""` | Exclude object types matching pattern |
| `--acl-files` | `false` | Save ACLs to separate `.acl.sql` files |
| `--move-roles` | `false` | Move role files under database directory |
| `--quiet` | `false` | Suppress informational output |
| `--dry-run` | `false` | Print what would be extracted without writing |
| `--version` | — | Print version and exit |
| `--test-sql` | `""` | (testing) Read SQL file directly (no pg_restore) |

## Testing

```bash
make test       # 27 unit tests (no PostgreSQL required)
make coverage   # 73% coverage (main() integration paths need pg_restore)
```

Tests use `--test-sql` + embedded SQL fixture to bypass pg_restore, and
`osExit` overrides to capture exit codes from `main()`.

## Archive

V1 (text-dump parser, cobra, yaml config) and v2 bash reference are in
`archive/` (excluded from linting and gitignore).

## Performance

Tested on ~150 objects / 2226 lines SQL output:

| Version | Avg Time | vs pg_atropos |
|---------|----------|---------------|
| **pg_atropos (Go)** | **0.044s** | **1×** |
| pgdump_splitter (Go) | 0.109s | 2.5× slower |

## Commit Conventions

Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages:

```
feat: add new feature
fix: fix a bug
docs: update documentation
refactor: restructure code
test: add or fix tests
ci: CI/CD changes
chore: maintenance tasks
```

These are used by `gh release --generate-notes` to categorize commits in release notes.

## Key Design Decisions

- Uses `pg_restore -f -` pipe instead of parsing text dumps directly
- No external dependencies (stdlib `flag` package instead of Cobra)
- No INDEX/CONSTRAINT/TRIGGER merging in custom mode (pg_restore headers don't contain parent table name)
- Streaming parser: no temp SQL files, no memory issues on large dumps
- Stdin pipe (`-f -`) enables `pg_dump -Fc ... | pg_atropos -f -` without intermediate file
- `osExit` variable replaces `os.Exit` for testability
- Local `flag.FlagSet` (not global) to avoid flag redefinition panics in tests
