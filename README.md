# rtm-gen-go

Code generator that produces a Go client library and a
[cobra](https://github.com/spf13/cobra) CLI for the
[Remember The Milk API](https://www.rememberthemilk.com/services/api/)
from a single spec snapshot (`api.json`).

The output is two separate Go modules, published independently:

- **`github.com/morozov/rtm-client-go`** — API client library
  (package `rtm`, standard library only).
- **`github.com/morozov/rtm-cli-go`** — cobra-based CLI; depends
  on `rtm-client-go`.

This repository holds the generator itself plus the committed
`api.json` it reads.

## Status

The `client` and `cli` generator subcommands are implemented and
covered by integration tests that build freshly-generated modules
in temp directories. The output modules have not yet been
published, so today the practical workflow is to generate locally
and stitch with a `replace` directive; see below.

Live spec fetch (generator → RTM directly) is behind the
`livefetch` build tag and requires a local sibling checkout of
`rtm-client-go`. The default build, default test suite, and CI
path do not require it.

## Using the generator

Install from source:

```sh
go install github.com/morozov/rtm-gen-go/cmd/rtm-gen@latest
```

Or run directly against a clone:

```sh
go run ./cmd/rtm-gen <subcommand> [flags]
```

Both subcommands default to writing into `generated/` (gitignored
in this repo), and default to the published module paths.

### Generate the client module

From a local spec file:

```sh
rtm-gen client \
  -spec ./path/to/api.json \
  -out generated/rtm-client-go \
  -module github.com/morozov/rtm-client-go \
  -package rtm
```

From a live RTM fetch (requires the `livefetch` build tag and a
sibling `../rtm-client-go/` checkout; see
[Live-fetch setup](#live-fetch-setup) below):

```sh
go run -tags=livefetch ./cmd/rtm-gen client \
  -key $RTM_API_KEY -secret $RTM_API_SECRET \
  -out generated/rtm-client-go
```

Writes `go.mod`, a core `client.go`, and one file per RTM service.

### Generate the CLI module

```sh
rtm-gen cli \
  -out generated/rtm-cli-go \
  -module github.com/morozov/rtm-cli-go \
  -package rtmcli \
  -client-module github.com/morozov/rtm-client-go \
  -client-package rtm
```

The CLI requires the client module. For local development (until
`rtm-client-go` is published), point at your local client output:

```sh
(cd generated/rtm-cli-go && \
  go mod edit -replace=github.com/morozov/rtm-client-go=../rtm-client-go && \
  go mod tidy)
```

After that, `go build ./cmd/rtm` inside the CLI module produces
the `rtm` binary.

### Live-fetch setup

The `livefetch` build tag enables the generator to pull the RTM
spec straight from RTM at generation time, using the pinned
`rtm-client-go` module. Because `rtm-client-go` is not yet
published, the build is wired via a local `replace` pointing at
`../rtm-client-go/`. To enable:

```sh
# From the parent directory of this repo:
go run ./rtm-gen-go/cmd/rtm-gen client \
  -spec ./rtm-gen-go/api.json \
  -out ./rtm-client-go

# Then, back in rtm-gen-go, live fetch is available:
go run -tags=livefetch ./cmd/rtm-gen client \
  -key $RTM_API_KEY -secret $RTM_API_SECRET \
  -out generated/rtm-client-go
```

Without the sibling directory:
- Default `go build`, `go test`, and `go tool golangci-lint` all
  still work.
- `go mod tidy` and `-tags=livefetch` builds fail because the
  `replace` target is missing.

Once `rtm-client-go` is published, the `replace` directive will
be dropped in favour of a normal `require` on the tagged
version, and the sibling requirement will go away.

## Using the generated client

```go
package main

import (
    "context"
    "fmt"

    "github.com/morozov/rtm-client-go"
)

func main() {
    c := rtm.NewClient("API_KEY", "API_SECRET", "AUTH_TOKEN")

    body, err := c.Lists.GetList(context.Background())
    if err != nil {
        panic(err)
    }
    fmt.Println(string(body))
}
```

Every RTM service is a field on `*rtm.Client`. Methods take a
`context.Context`, and a typed `<Service><Method>Params` struct
when the method has user-facing arguments. Required fields are
plain `string`; optional fields are `*string`.

`api_key`, `auth_token`, `timeline`, and `api_sig` are injected
automatically — the caller only supplies RTM-specific arguments.
`ErrMissingAuthToken` and `ErrMissingTimeline` signal the cases
where the client was not configured with the required credentials
for a particular call.

The client returns the raw HTTP response body. Parsing the RTM
response (XML by default, JSON with `?format=json`) is the
caller's responsibility.

## Using the generated CLI

Install:

```sh
go install github.com/morozov/rtm-cli-go/cmd/rtm@latest
```

Or after a local generate, from inside `generated/rtm-cli-go/`:

```sh
go build ./cmd/rtm
./rtm --help
```

Invoke:

```sh
rtm --key $RTM_API_KEY --secret $RTM_API_SECRET --token $RTM_AUTH_TOKEN \
    <service> <method> [flags]
```

The CLI mirrors the RTM service hierarchy. Representative
commands:

```sh
rtm reflection get-methods
rtm auth check-token
rtm lists get-list
rtm tasks add --name "Ship it" --list-id 123
rtm tasks notes add --list-id 1 --taskseries-id 2 --task-id 3 --note-title "..."
```

`--key` and `--secret` are required for every invocation;
`--token` is required for methods that need a logged-in user.

## Specs

Authoritative design lives under [`specs/`](./specs). Start with
[`specs/INDEX.md`](./specs/INDEX.md);
[spec 001](./specs/001-codegen-architecture.md) is the canonical
architecture reference.
