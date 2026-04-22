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

RTM publishes its API shape only through its own reflection
endpoints (`rtm.reflection.getMethods` plus per-method
`rtm.reflection.getMethodInfo`) — there is no separate schema
file the generator can read offline. The generator therefore
needs a copy of that reflection data at generation time, and
accepts it in one of two forms. Both subcommands (`client` and
`cli`) accept the same pair.

- **Local file** (`--spec=./api.json`) — a JSON document
  holding a prior capture of the reflection responses. Use it
  for CI, scripted regeneration, and any build where
  reproducibility and offline operation matter more than being
  up-to-the-minute with RTM. rtm-cli-go's default build takes
  this route.
- **Live RTM fetch** (`--key=… --secret=…`) — `rtm-gen` calls
  the reflection endpoints directly on every run. Use it for
  first-time generation or to refresh a cached spec against
  newly added RTM methods or fields.

From a local spec file:

```sh
rtm-gen client \
  --spec=./path/to/api.json \
  --out=path/to/rtm-cli-go/internal/rtm
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
  --out=path/to/rtm-cli-go/internal/commands
```

Emits `register.go` (the `Register(root, provider, formatter)`
entry point) plus one file per RTM service. The remaining flags
(`--package`, `--client-module`, `--client-package`) default to
values that suit `rtm-cli-go`; a different host module needs to
override `--client-module` so the generated commands import from
the right Go package path.

### From the consumer side

`rtm-cli-go` pins the generator via `tool` directive in its
`go.mod` and drives regeneration with `//go:generate` anchor
files at `internal/rtm/gen.go` and `internal/commands/gen.go`:

```sh
go generate ./...
```

## Generated output contract

- The **client** package exposes typed `*<Service><Method>Params`
  and `*<Service><Method>Response` structs — no
  `json.RawMessage` on the happy path — plus enum aliases
  (`Priority`, `Perms`, `Direction`) and an exported
  `Sign(url.Values) string` for hosts that build their own
  signed URLs. The client unwraps the RTM envelope; `APIError`
  (wrapping `ErrRTMAPI`) carries the upstream code and message.
- The **commands** package exposes one entry point,
  `Register(root *cobra.Command, provider ClientProvider,
  format Formatter)`. Typed flags validate locally; enum flags
  register shell completion and carry an `rtm-gen.enum` pflag
  annotation so a manifest-style subcommand can enumerate the
  legal values without scraping help text.

The host module owns everything else — persistent flags,
credential sourcing, output format, config, and any bespoke
commands that aren't RTM API bindings.

For a worked example, see
[rtm-cli-go](https://github.com/morozov/rtm-cli-go)'s
`cmd/rtm/main.go` and `internal/rtm/`.
