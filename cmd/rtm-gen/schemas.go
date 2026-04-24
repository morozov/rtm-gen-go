package main

import (
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	"github.com/morozov/rtm-gen-go/internal/gen"
)

func runSchemas(args []string) error {
	fs := pflag.NewFlagSet("schemas", pflag.ContinueOnError)
	specPath := fs.String("spec", "", "path to a local RTM reflection dump (mutually exclusive with --key/--secret)")
	apiKey := fs.String("key", "", "RTM API key for live spec fetch (requires --secret; mutually exclusive with --spec)")
	apiSecret := fs.String("secret", "", "RTM API secret for live spec fetch")
	outDir := fs.String("out", "generated/schemas", "output directory for the emitted JSON Schema files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	}

	spec, err := loadSpec(*specPath, *apiKey, *apiSecret)
	if err != nil {
		return err
	}
	files, err := gen.GenerateSchemas(spec, gen.SchemaConfig{OutDir: *outDir})
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Println(f)
	}
	return nil
}
