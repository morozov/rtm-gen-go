// Command rtm-gen reads an RTM reflection dump and emits Go
// packages for consumption by a hand-written CLI module.
// Subcommands: `spec` fetches the reflection dump from RTM,
// `client` emits the RTM API client package, `cli` emits the
// cobra commands package.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "rtm-gen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("no subcommand given")
	}
	switch args[0] {
	case "spec":
		return runSpec(args[1:])
	case "client":
		return runClient(args[1:])
	case "cli":
		return runCLI(args[1:])
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

const usageText = `usage: rtm-gen <subcommand> [flags]

subcommands:
  spec      fetch the RTM reflection dump and write it as JSON
  client    generate the RTM API client package
  cli       generate the cobra commands package
`

func usage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}
