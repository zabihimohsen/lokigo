# Contributing to lokigo

Thanks for contributing.

## Development setup

```bash
git clone git@github.com:zabihimohsen/lokigo.git
cd lokigo
go test ./...
go test -race ./...
go vet ./...
```

## Pull request expectations

- Keep PRs focused and small when possible.
- Add or update tests for behavior changes.
- Update docs and `CHANGELOG.md` for user-facing changes.
- Use clear PR titles (conventional style is welcome but not required).

## Release notes hygiene

If your change is user-visible, include a short changelog-ready summary in the PR description.

## Reporting bugs

Please include:

- Go version
- lokigo version/tag
- minimal reproduction snippet
- expected vs actual behavior
- Loki endpoint type (self-hosted, Grafana Cloud, etc.)

## Code style

- Run `gofmt` (or `go test` which may reveal formatting/test issues).
- Prefer explicit, readable APIs over clever abstractions.
- Keep label-cardinality safety top of mind for slog integration changes.
