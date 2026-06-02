# Contributing

Thanks for your interest in pg_atropos!

## Prerequisites

- Go 1.23+
- `golangci-lint` (optional, for `make lint`)
- PostgreSQL client tools (`pg_dump`, `pg_restore`) — only needed for
  integration tests or real usage

## Project Structure

- `main.go` — single-file implementation (no packages, no external deps)
- `main_test.go` — unit tests using embedded SQL fixture + `osExit` override
- `archive/` — historical v1 (Go + Cobra) and v2 bash reference (excluded from lint)

## Development workflow

```bash
make build      # Build for current platform
make test       # Run unit tests (no database required)
make coverage   # Unit tests with coverage report
make lint       # Run golangci-lint
make clean      # Remove build artifacts
```

Tests use `--test-sql` flag to bypass `pg_restore` — no PostgreSQL needed
for unit tests. Coverage is ~73%; the uncovered paths (`main()`, 
`detectDbname()`, `checkDep()` error paths) require `pg_dump`/`pg_restore`.

## Code style

- Go code is formatted with `gofmt -s` (enforced by CI)
- No comments in production code (unless the logic genuinely needs explanation)
- Single `main.go` file — no package extraction unless necessary
- Use `osExit` variable instead of `os.Exit` for testability
- Use local `flag.FlagSet` (not global) to avoid flag redefinition panics
- Keep external dependencies at zero (stdlib only)

## Testing guidelines

- Add table-driven tests for parsing functions (`parseHeader`, `checkDbFilter`, …)
- For `parseAndWrite` / `writeObject` tests, use the embedded `fullRestoreFixture`
- For `main()` tests, override `osExit` to capture exit codes
- Prefer `t.TempDir()` over manual temp directory management
- Don't add tests that require a running PostgreSQL — those go in CI only

## How to submit changes

1. Fork the repo and create a feature branch
2. Make your changes
3. Run `make lint && make test` — both must pass
4. Open a pull request with a clear description of what and why

## Versioning

The project follows [SemVer](https://semver.org/). Version is stored in
`VERSION` file and embedded at build time via `-ldflags "-X main.version=$(VERSION)"`.

## License

By contributing, you agree that your contributions will be licensed under
the MIT License (see [LICENSE](LICENSE)).
