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
  `mise run api:breaking` checks this against `main`.
- Money is a decimal string plus a currency code. Never use float types for money.

## Code layout and rules

```text
cmd/orders/              wiring only
internal/httpapi/        HTTP: implements api.StrictServerInterface, maps errors
internal/order/          domain: types, Service, Repository interface; no HTTP, no storage
internal/storage/        Repository implementations
```

- Imports go one way: `cmd` → `httpapi` → `order` ← `storage`.
  `mise run arch` checks this (`.go-arch-lint.yml`).
- Business logic goes into `internal/order`, not into handlers.
- A new endpoint: add it to the spec, run `mise run gen`, fix the compile error
  by implementing the new method in `internal/httpapi`, add tests.
- Tests in `internal/httpapi` go through the full HTTP stack and check every
  response against the spec with `assertMatchesSpec`. Keep that for new endpoints.

## Ask a human first

Do not do these without asking:

- edit `.golangci.yml`, `.go-arch-lint.yml`, the checker versions in `mise.toml`,
  or `.github/workflows/`;
- add `//nolint`, betterleaks allow comments, or skip a check in any other way;
- add a new third-party Go module or npm package;
- make a breaking API change.

If a check fails, fix the code, not the check.
