# pg_atropos

[![Go](https://img.shields.io/badge/Go-1.23-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CLI Tool](https://img.shields.io/badge/Interface-CLI%20Wrapper-green.svg)](pg_atropos)

Split PostgreSQL `pg_dump -Fc` (custom-format) dumps into individual files
organized for GIT versioning.

## Why?

When a team shares a single giant SQL dump in Git, every change — adding a
column, a new index, tweaking a function — creates a merge conflict across
the entire file. Resolving those conflicts is tedious and error-prone.

`pg_atropos` splits the dump into individual objects (tables, functions,
indexes, roles, …), each in its own file. In Git this means:

- **Minimal conflicts** — two people can change different tables without
  stepping on each other
- **Clear code review** — a diff shows exactly the changed object, not
  500 lines of dump header
- **Readable history** — `git log -- TABLE/users.sql` shows every change
  to that specific table
- **CI/CD friendly** — deploy only the changed object, not the entire dump

## How?

Instead of parsing raw SQL text (brittle regex), `pg_atropos` pipes through
`pg_restore -f -` to reliably extract each object via its header metadata.

**Requirement:** `pg_restore` (from [PostgreSQL client tools](https://www.postgresql.org/download/))
must be installed. It is used to decompress and interpret the custom-format dump
— `pg_atropos` never reads the binary format directly.

## The Name

In Greek mythology, the three Moirai (Fates) spun the thread of life,
measured it, and — finally — **Atropos** (the Inevitable) cut it with
her shears.  `pg_atropos` does the same: it takes a giant `pg_dump`
file and snips it into small, manageable threads (files) ready for Git.
Who wouldn't want shears that cut a dump into pieces?

## Inspiration

This project was inspired by
[michal-bartak/pgdump_splitter](https://github.com/michal-bartak/pgdump_splitter),
which parses plain `pg_dump` text output.  That approach is fragile — object
boundaries are hard to detect reliably when function bodies or comments contain
text that looks like dump markers.  `pg_atropos` solves this by using the
**custom-format** (`-Fc`) dump + `pg_restore -f -` pipe, which emits clean
`-- Name: ...; Type: ...` headers that the parser can trust.

The result: simpler, faster, and far more robust parsing.

## Quick Start

```bash
# From a custom-format dump file
pg_atropos -f dump.pgdump -output ./structure

# From a live database (auto-dump via pg_dump)
pg_atropos -d mydb -output ./structure

# Pipe from pg_dump (no temp file needed)
pg_dump -Fc mydb -f - | pg_atropos -f - -output ./structure

# Pipe from a remote database via ssh
ssh dbserver 'pg_dump -Fc mydb -f -' | pg_atropos -f - -output ./structure

# Custom mode (lowercase directories for CI)
pg_atropos -f dump.pgdump -output ./structure -mode custom
```

## Installation

### From source

```bash
git clone https://github.com/your-project/pg_atropos.git
cd pg_atropos && make build
```

### Docker

```bash
docker build -t pg_atropos .
docker run --rm -v $(pwd)/dump.pgdump:/dump.pgdump pg_atropos -f /dump.pgdump
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | `""` | Database name to dump |
| `--conn` | `""` | PostgreSQL connection string |
| `--file`, `-f` | `""` | Custom-format dump file (`"-"` for stdin) |
| `--output` | `./output` | Output directory |
| `--mode` | `origin` | Output mode: `origin` \| `custom` |
| `--clean` | `false` | Clean output directory before processing |
| `--no-db-path` | `false` | Don't include database name in output path |
| `--blacklist-db` | `^(template\|postgres)` | Skip databases matching pattern |
| `--whitelist-db` | `""` | Only include databases matching pattern |
| `--exclude-obj` | `""` | Exclude object types matching pattern |
| `--acl-files` | `false` | Save ACLs to separate `.acl.sql` files |
| `--move-roles` | `false` | Move role files under database directory |
| `--dry-run` | `false` | Print what would be extracted without writing |
| `--quiet` | `false` | Suppress informational output |
| `--version` | — | Print version and exit |

## Modes

### origin (default)

Mirrors the dump structure exactly — each object type gets its own
directory (`TABLE/`, `FUNCTION/`, `INDEX/`, …).  Good for inspection.

### custom

Lowercase directories (`table/`, `function/`, …).  Indexes, constraints,
triggers are **not** merged into their parent table (pg_restore headers
don't carry the parent table name, so merge would require SQL content
parsing).  Better suited for CI / automation workflows.

## Tests

```bash
make test       # unit tests (no database required)
make coverage   # with coverage report
```

Tests use the `--test-sql` flag to inject pre-extracted SQL directly,
bypassing `pg_restore`.  No PostgreSQL installation needed.

## Limitations

- INDEX / CONSTRAINT / TRIGGER merging into table files is not supported
  in custom mode (pg_restore headers lack parent table name).
- Object names with spaces or special characters may not round-trip.

## Performance

~150 objects / 2226 SQL lines:

| Version | Avg Time | vs pg_atropos |
|---------|----------|---------------|
| **pg_atropos (Go)** | **0.044s** | **1×** |
| pgdump_splitter (Go) | 0.109s | 2.5× slower |

## Future Improvements

- **Merge INDEX/CONSTRAINT/TRIGGER under table/** — would require
  parsing the SQL content to identify the parent table (doable, adds
  complexity).
- **Memory limits** — configurable scanner buffer for constrained
  Docker environments.
- **Git integration** — commit each object separately with structured
  commit messages.

## License

MIT — see [LICENSE](LICENSE)
