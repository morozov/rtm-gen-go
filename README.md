# rtm-gen-go

Code generator that produces two Go packages for talking to the
[Remember The Milk API](https://www.rememberthemilk.com/services/api/):
a stdlib-only **client** and a [cobra](https://github.com/spf13/cobra)
**commands** tree.

The generator's output is consumed by a separate hand-written CLI
repository (`rtm-cli-go`) that commits the generated code under
`internal/rtm/` and `internal/commands/` and owns the rest —
`main.go`, root cobra command, persistent flags, credential
sourcing, output formatting.

This repository holds the generator itself and nothing generated.

## Status

The `client` and `cli` subcommands are implemented and covered by
integration tests that generate both packages into a temp module,
wire them with a hand-rolled `go.mod` + `main.go`, and run the
boundary suite. Live spec fetch runs directly through stdlib
`net/http`; no sibling checkout or build tag required.

See [`specs/`](./specs) for the authoritative design. Start with
[`specs/INDEX.md`](./specs/INDEX.md);
[spec 001](./specs/001-codegen-architecture.md) is the canonical
architecture reference.

## Using the generator

Install:

```sh
go install github.com/morozov/rtm-gen-go/cmd/rtm-gen@latest
```

Or run directly from a clone:

```sh
go run ./cmd/rtm-gen <subcommand> [flags]
```

### Generate the client package

From a local spec file:

```sh
rtm-gen client \
  -spec ./path/to/api.json \
  -out path/to/rtm-cli-go/internal/rtm \
  -package rtm
```

From a live RTM fetch:

```sh
rtm-gen client \
  -key $RTM_API_KEY -secret $RTM_API_SECRET \
  -out path/to/rtm-cli-go/internal/rtm
```

Emits `client.go` plus one file per RTM service. The generator
does **not** emit a `go.mod` — the client package lives as a
subpackage inside the host CLI module.

### Generate the commands package

```sh
rtm-gen cli \
  -spec ./path/to/api.json \
  -out path/to/rtm-cli-go/internal/commands \
  -package commands \
  -client-module github.com/morozov/rtm-cli-go/internal/rtm \
  -client-package rtm
```

Emits `register.go` (the `Register(root, provider)` entry point)
plus one file per RTM service.

### From the consumer side

`rtm-cli-go` pins the generator via `tool` directive in its
`go.mod` and drives regeneration with `//go:generate` anchor
files at `internal/rtm/gen.go` and `internal/commands/gen.go`:

```sh
go generate ./...
```

## Using the generated client

Inside the host module:

```go
package main

import (
    "context"
    "fmt"

    "github.com/morozov/rtm-cli-go/internal/rtm"
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
`context.Context` and a typed `<Service><Method>Params` struct
when the method has user-facing arguments. Required fields are
plain `string`; optional fields are `*string`.

`api_key`, `auth_token`, `timeline`, and `api_sig` are injected
automatically — the caller only supplies RTM-specific arguments.
`ErrMissingAuthToken` and `ErrMissingTimeline` signal the cases
where the client was not configured with the required credentials
for a particular call.

Every request uses `format=json`; the raw HTTP response body is
returned verbatim. Parsing the RTM envelope is the caller's
responsibility.

## Using the generated commands

The host's `cmd/rtm/main.go` mounts the generated commands onto
its own cobra root:

```go
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/morozov/rtm-cli-go/internal/commands"
    "github.com/morozov/rtm-cli-go/internal/rtm"
)

func main() {
    var apiKey, apiSecret, authToken string
    var client *rtm.Client

    root := &cobra.Command{
        Use:   "rtm",
        Short: "Remember The Milk CLI",
        PersistentPreRunE: func(*cobra.Command, []string) error {
            client = rtm.NewClient(apiKey, apiSecret, authToken)
            return nil
        },
    }
    root.PersistentFlags().StringVar(&apiKey, "key", "", "RTM API key")
    root.PersistentFlags().StringVar(&apiSecret, "secret", "", "RTM API secret")
    root.PersistentFlags().StringVar(&authToken, "token", "", "RTM auth token")

    commands.Register(root, func() *rtm.Client { return client })

    if err := root.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Once built:

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

The host owns flag parsing, so `--key`/`--secret`/`--token`
behaviour is defined by the host's `main.go`, not the generator.
