# rtm-gen-go

Code generator that produces two Go packages for talking to the
[Remember The Milk API](https://www.rememberthemilk.com/services/api/):
an stdlib-only **client** and a [cobra](https://github.com/spf13/cobra)
**commands** tree.

The generator's output is consumed by a host module; the
reference host is
[rtm-cli-go](https://github.com/morozov/rtm-cli-go).

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
  --spec=./path/to/api.json \
  --out=path/to/rtm-cli-go/internal/rtm \
  --package=rtm
```

From a live RTM fetch:

```sh
rtm-gen client \
  --key=$RTM_API_KEY --secret=$RTM_API_SECRET \
  --out=path/to/rtm-cli-go/internal/rtm
```

Emits `client.go` plus one file per RTM service. The generator
does **not** emit a `go.mod` — the client package lives as a
subpackage inside the host CLI module.

### Generate the commands package

```sh
rtm-gen cli \
  --spec=./path/to/api.json \
  --out=path/to/rtm-cli-go/internal/commands \
  --package=commands \
  --client-module=github.com/morozov/rtm-cli-go/internal/rtm \
  --client-package=rtm
```

Emits `register.go` (the `Register(root, provider, formatter)`
entry point) plus one file per RTM service.

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

    resp, err := c.Lists.GetList(context.Background())
    if err != nil {
        panic(err)
    }
    for _, l := range resp.Lists.List {
        fmt.Printf("%d %s archived=%t\n", l.ID, l.Name, l.Archived)
    }
}
```

Every RTM service is a field on `*rtm.Client`. Methods take a
`context.Context` (and a typed `<Service><Method>Params` struct
when the method has user-facing arguments) and return a typed
`*<Service><Method>Response` — never `json.RawMessage`.

- Scalar fields carry semantic Go types: integer IDs are `int64`
  via the `rtmInt` wrapper, booleans are `bool` via `rtmBool`,
  timestamps are `time.Time` via `rtmTime` (with `Valid` for
  RTM's `""` → absent convention).
- Enum fields use catalogue-backed string aliases (`Priority`,
  `Perms`, `Direction`) with named constants (`Priority1`,
  `PermsRead`, …). Unknown wire values round-trip through the
  alias unchanged.
- `Params` fields follow the same typing on the request side:
  required args are typed scalars (`int64`, `bool`, enum
  aliases, `[]string` for comma-lists), optional args are
  pointers to the same.

`api_key`, `auth_token`, `timeline`, and `api_sig` are injected
automatically — the caller only supplies RTM-specific arguments.
`ErrMissingAuthToken` and `ErrMissingTimeline` signal the cases
where the client was not configured with the required credentials
for a particular call; `APIError` (wrapping the sentinel
`ErrRTMAPI`) carries RTM's own code and message. The client
unwraps the RTM envelope before handing the response back — the
caller never sees the `rsp`/`stat` layer.

`Sign(url.Values) string` is exposed for host code that needs to
build signed URLs outside of `Call` (e.g., the browser approval
URL during an auth-login flow).

## Using the generated commands

The host's `cmd/rtm/main.go` mounts the generated commands onto
its own cobra root. `Register` takes a `ClientProvider` (called
lazily so persistent flags are populated before the client is
built) and a `Formatter` (writes a typed response to `io.Writer`):

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
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
    root.PersistentFlags().StringVar(&authToken, "token", "", "RTM auth token (optional)")

    commands.Register(
        root,
        func() *rtm.Client { return client },
        func(w io.Writer, body any) error {
            enc := json.NewEncoder(w)
            enc.SetIndent("", "  ")
            return enc.Encode(body)
        },
    )

    if err := root.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Once built:

```sh
rtm --key=$RTM_API_KEY --secret=$RTM_API_SECRET --token=$RTM_AUTH_TOKEN \
    <service> <method> [flags]
```

The CLI mirrors the RTM service hierarchy. Representative
commands:

```sh
rtm reflection get-methods
rtm auth check-token
rtm lists get-list
rtm tasks add --name="Ship it" --list-id=123
rtm tasks set-priority --list-id=1 --taskseries-id=2 --task-id=3 --priority=2
rtm tasks set-tags --list-id=1 --taskseries-id=2 --task-id=3 --tags=shipit,work
rtm tasks notes add --list-id=1 --taskseries-id=2 --task-id=3 --note-title="..."
```

Typed flags mean local validation: `--list-id=foo` fails before
any HTTP call; `--priority=banana` fails with
"expected 1, 2, 3, N". Enum flags also register shell completion
and carry an `rtm-gen.enum` pflag annotation so programmatic
consumers can enumerate the legal values.

The host owns flag parsing, so `--key`/`--secret`/`--token`
behaviour — and any additional concerns like config files, env
vars, output formats, or bespoke commands (e.g., an `auth login`
flow) — live in the host module, not in the generator.
