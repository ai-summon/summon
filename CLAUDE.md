# CLAUDE.md

## Testing Requirements

A feature is not complete until:

1. Every new or modified behavior has a corresponding test (new test or updated existing test).
2. The full test suite passes: `go test ./... -count=1`
3. Relevant documentation in `docs/` is updated to reflect the changes.
4. The binary is rebuilt so it stays current: `go build -o ./bin/summon ./cmd/summon`

## Commit Policy

Do not commit code changes.
The user will perform all commits after reviewing the changes.
