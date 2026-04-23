package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/morozov/rtm-gen-go/internal/fetch"
)

func runSpec(args []string) error {
	fs := pflag.NewFlagSet("spec", pflag.ContinueOnError)
	apiKey := fs.String("key", "", "RTM API key (required)")
	apiSecret := fs.String("secret", "", "RTM API secret (required)")
	outPath := fs.String("out", "", "output file path (default: stdout)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	}
	if *apiKey == "" || *apiSecret == "" {
		return fmt.Errorf("--key and --secret are required")
	}

	raw, err := fetch.FetchRaw(context.Background(), *apiKey, *apiSecret, "")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return fmt.Errorf("format spec: %w", err)
	}
	buf.WriteByte('\n')

	if *outPath == "" {
		_, err := os.Stdout.Write(buf.Bytes())
		return err
	}
	return os.WriteFile(*outPath, buf.Bytes(), 0o644)
}
