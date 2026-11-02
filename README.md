# typespec-http

Orders API: a Go HTTP server generated from a TypeSpec spec.

The API contract is in [spec/main.tsp](spec/main.tsp).

## Setup

Supported: macOS (arm64) and Debian Linux (x64, arm64). All tools come from
[mise](https://mise.jdx.dev). Exact versions and checksums are in `mise.lock`.

1. System packages.

   macOS:

   ```sh
   brew install mise
   ```

   Debian (the Node binary needs `libatomic1`):

   ```sh
   sudo apt-get install -y curl ca-certificates git libatomic1
   curl -fsSL https://mise.run | sh
   ```

2. Activate mise in your shell (once):

   ```sh
   echo 'eval "$(mise activate zsh)"' >> ~/.zshrc   # or bash: ~/.bashrc
   ```

3. Install the project tools and packages:

   ```sh
   mise trust
   mise install --locked
   mise run setup
   ```

## Tasks

```sh
mise tasks           # list all tasks
mise run spec:check  # check the TypeSpec spec
mise run gen:openapi # generate api/openapi.yaml
mise run gen:go      # generate internal/api/api.gen.go
mise run gen         # both, in this order
mise run test        # Go tests with the race detector
mise run build       # build bin/orders
mise run run         # run the server on :8080 (set ADDR to change)
```

Try it:

```sh
curl localhost:8080/orders/1
```

## Code layout

```text
cmd/orders/              main: config, wiring, graceful shutdown
internal/api/            GENERATED: types, router, strict server interface
internal/httpapi/        HTTP layer: request validation, handler, error bodies
internal/order/          domain: Order, Service, Repository interface
internal/storage/memory/ in-memory Repository (demo data until there is a DB)
```

Dependencies go one way: `cmd` → `httpapi` → `order` ← `storage`.
Requests are checked against the OpenAPI spec before they reach the handler,
and the tests check every response against the spec too.

## Generated files

```text
spec/main.tsp  --tsp-->  api/openapi.yaml  --oapi-codegen-->  internal/api/api.gen.go
```

Do not edit generated files by hand: change `spec/main.tsp` and run `mise run gen`.
Both files are committed, so reviews show API changes and breaking changes can be
checked against `main`.

oapi-codegen is pinned in `go.mod` as a Go tool (`go tool oapi-codegen`), its
settings are in `oapi-codegen.yaml`.

## Updating tool versions

Change the version in `mise.toml`, then run `mise lock` and commit `mise.lock`.
It keeps entries for every platform in `lockfile_platforms`.
