// Command rtm-gen reads an RTM reflection dump and emits the Go
// modules described by this project's specs. It has one subcommand
// per target module: `client` emits rtm-client-go, `cli` emits
// rtm-cli-go.
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
  client    generate the rtm-client-go module
  cli       generate the rtm-cli-go module
`

func usage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}
