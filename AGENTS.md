# Agent instructions

Orders API: a Go HTTP server generated from a TypeSpec spec. Details are in
[README.md](README.md).

## Commands

All tools come from mise. Do not install tools in other ways.

```sh
mise install --locked && mise run setup   # once
mise run gen      # after changing spec/main.tsp
mise run test     # after changing Go code
mise run check    # before you say the work is done; it must exit 0
```

`mise tasks` lists every task. To force a task, put `--force` before the task
name: `mise run --force gen`. After the task name it goes to the command instead.

## The API contract

- `spec/main.tsp` is the only source of the API. To change the API, change it,
  then run `mise run gen`.
- Generated files, never edit them by hand:
  - `api/openapi.yaml`
  - `internal/api/api.gen.go`

  `mise run gen:check` fails on hand edits and on stale files.
- Do not make breaking API changes (removing or renaming a field, changing a
  type, making an optional field required) unless a human asked for it.
  `mise run api:breaking` checks this against the commit where the branch left `main`.
- Money is a decimal string plus a currency code. Never use float types for money.

## Code layout and rules

```text
cmd/orders/              wiring only
internal/httpapi/        HTTP: implements api.StrictServerInterface, maps errors
internal/order/          domain: types, Service, Repository interface; no HTTP, no storage
internal/storage/        Repository implementations
```

- Imports go one way: `cmd` → `httpapi` → `order` ← `storage`, as drawn in
  [ARCHITECTURE.md](ARCHITECTURE.md). `mise run arch` (`.go-arch-lint.yml`) and
  `mise run easement` (`easement.yml`) check this.
- Business logic goes into `internal/order`, not into handlers. The domain does
  not call `time.Now()`: it gets the time from its caller.
- Log with `log/slog` only. No `fmt.Print*`, no `log.*`, no `http.DefaultClient`.
- Every package has tests (`mise run test:presence`), except `package main` and
  generated code.
- A new endpoint: add it to the spec, run `mise run gen`, fix the compile error
  by implementing the new method in `internal/httpapi`, add tests.
- Tests in `internal/httpapi` go through the full HTTP stack and check every
  response against the spec with `assertMatchesSpec`. Keep that for new endpoints.

## Before and after a change

easement answers questions about the code graph:

```sh
easement explain internal/httpapi/handler.go   # its component, and what it may import
easement why httpapi order                     # is httpapi -> order allowed, and which imports make it
mise run impact                                # files that depend on what you changed: run their tests
```

Run `explain` before you add a file or an import. Note: in Go, easement follows
an import to one file of the package, so `why` and `impact` can miss a chain that
goes through another file of the same package.

## Ask a human first

Do not do these without asking:

- edit `.golangci.yml`, `.go-arch-lint.yml`, `easement.yml`, the diagram in
  `ARCHITECTURE.md`, the checker versions in `mise.toml`, or `.github/workflows/`;
- add `//nolint`, `easement:disable`, betterleaks allow comments, or skip a check
  in any other way. If a human agrees, the form is
  `//nolint:<linter> // <reason>`; a bare `//nolint` fails the lint;
- add a new third-party Go module or npm package;
- make a breaking API change.

If a check fails, fix the code, not the check.
