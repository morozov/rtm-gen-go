package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSpecFlagValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		spec    string
		key     string
		secret  string
		wantSub string
	}{
		{"no inputs", "", "", "", "either --spec"},
		{"both spec and creds", "/tmp/x.json", "k", "s", "mutually exclusive"},
		{"spec set with key only", "/tmp/x.json", "k", "", "mutually exclusive"},
		{"spec set with secret only", "/tmp/x.json", "", "s", "mutually exclusive"},
		{"key without secret", "", "k", "", "both --key and --secret"},
		{"secret without key", "", "", "s", "both --key and --secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadSpec(tc.spec, tc.key, tc.secret)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoadSpecReadsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	const fixture = `{
		"rtm.alpha.ping": {
			"name": "rtm.alpha.ping",
			"needslogin": "0",
			"needssigning": "0",
			"requiredperms": "0",
			"description": "x",
			"response": "<rsp stat=\"ok\"/>",
			"arguments": {"argument": [{"name": "api_key", "optional": "0", "$t": "k"}]},
			"errors": {"error": []}
		}
	}`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	spec, err := loadSpec(path, "", "")
	if err != nil {
		t.Fatalf("loadSpec: %v", err)
	}
	if _, ok := spec["rtm.alpha.ping"]; !ok {
		t.Fatalf("loaded spec missing expected method: %v", spec)
	}
}

func TestRunDispatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"no args", nil, "no subcommand"},
		{"unknown sub", []string{"frobnicate"}, "unknown subcommand"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := run(tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestRunHelpReturnsNil(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"-h", "--help", "help"} {
		if err := run([]string{flag}); err != nil {
			t.Fatalf("%s should be a no-op, got %v", flag, err)
		}
	}
}

func TestRunSpecRequiresCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{"no flags", nil},
		{"key only", []string{"--key", "k"}},
		{"secret only", []string{"--secret", "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := runSpec(tc.args)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "--key and --secret are required") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
