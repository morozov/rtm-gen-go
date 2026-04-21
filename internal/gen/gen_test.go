package gen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morozov/rtm-gen-go/internal/apispec"
	"github.com/morozov/rtm-gen-go/internal/gen"
	"github.com/morozov/rtm-gen-go/internal/naming"
)

const committedDumpPath = "../../api.json"

func defaultConfig(outDir string) gen.Config {
	return gen.Config{
		OutDir:      outDir,
		ModulePath:  "example.com/rtm",
		PackageName: "rtm",
		GoVersion:   "1.26",
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	filesA, err := gen.GenerateClient(spec, defaultConfig(dir1))
	require.NoError(t, err)
	filesB, err := gen.GenerateClient(spec, defaultConfig(dir2))
	require.NoError(t, err)
	require.Len(t, filesA, len(filesB))

	for i := range filesA {
		relA, err := filepath.Rel(dir1, filesA[i])
		require.NoError(t, err)
		relB, err := filepath.Rel(dir2, filesB[i])
		require.NoError(t, err)
		assert.Equal(t, relA, relB, "generator should emit the same file names in the same order")

		contentA, err := os.ReadFile(filesA[i])
		require.NoError(t, err)
		contentB, err := os.ReadFile(filesB[i])
		require.NoError(t, err)
		assert.Equal(t, contentA, contentB, "file %s differs between runs", relA)
	}
}

func TestGenerateEmitsGomodAndPerService(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)

	dir := t.TempDir()
	files, err := gen.GenerateClient(spec, defaultConfig(dir))
	require.NoError(t, err)

	names := make(map[string]bool, len(files))
	for _, p := range files {
		names[filepath.Base(p)] = true
	}
	assert.True(t, names["go.mod"], "go.mod must be emitted")
	assert.True(t, names["client.go"], "core file must be emitted")
	for _, svc := range []string{"auth.go", "reflection.go", "tasks.go", "tasks_notes.go"} {
		assert.Truef(t, names[svc], "service file %s must be emitted", svc)
	}
}

func TestGenerateRejectsBadMethodName(t *testing.T) {
	t.Parallel()

	spec := apispec.Spec{
		"not.rtm.prefixed": apispec.Method{Name: "not.rtm.prefixed"},
	}
	_, err := gen.GenerateClient(spec, defaultConfig(t.TempDir()))
	require.Error(t, err)
}

func TestGenerateRequiresConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  gen.Config
	}{
		{"missing OutDir", gen.Config{ModulePath: "m", PackageName: "p", GoVersion: "1.26"}},
		{"missing ModulePath", gen.Config{OutDir: t.TempDir(), PackageName: "p", GoVersion: "1.26"}},
		{"missing PackageName", gen.Config{OutDir: t.TempDir(), ModulePath: "m", GoVersion: "1.26"}},
		{"missing GoVersion", gen.Config{OutDir: t.TempDir(), ModulePath: "m", PackageName: "p"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := gen.GenerateClient(apispec.Spec{}, tc.cfg)
			require.Error(t, err)
			assert.ErrorIs(t, err, gen.ErrInvalidConfig)
		})
	}
}

func defaultCLIConfig(outDir string) gen.CLIConfig {
	return gen.CLIConfig{
		OutDir:            outDir,
		ModulePath:        "test.local/rtmcli",
		PackageName:       "rtmcli",
		GoVersion:         "1.26",
		ClientModulePath:  "test.local/rtmclient",
		ClientPackageName: "rtm",
		ClientVersion:     "v0.0.1",
		CobraVersion:      "1.8.1",
	}
}

func TestGenerateCLIIsDeterministic(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	filesA, err := gen.GenerateCLI(spec, defaultCLIConfig(dir1))
	require.NoError(t, err)
	filesB, err := gen.GenerateCLI(spec, defaultCLIConfig(dir2))
	require.NoError(t, err)
	require.Len(t, filesA, len(filesB))

	for i := range filesA {
		contentA, err := os.ReadFile(filesA[i])
		require.NoError(t, err)
		contentB, err := os.ReadFile(filesB[i])
		require.NoError(t, err)
		relA, _ := filepath.Rel(dir1, filesA[i])
		assert.Equal(t, contentA, contentB, "file %s differs between runs", relA)
	}
}

func TestGenerateCLIRequiresConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultCLIConfig(t.TempDir())
	cfg.ClientModulePath = ""
	_, err := gen.GenerateCLI(apispec.Spec{}, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, gen.ErrInvalidConfig)
}

// TestGeneratedCLIBuildsAndReachesAllCommands generates both modules,
// stitches them with a local replace, populates cobra via `go mod
// tidy`, and asserts the constructed command tree covers every RTM
// method listed in the committed spec.
func TestGeneratedCLIBuildsAndReachesAllCommands(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)

	root := t.TempDir()
	clientDir := filepath.Join(root, "client")
	cliDir := filepath.Join(root, "cli")
	require.NoError(t, os.MkdirAll(clientDir, 0o755))
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	_, err = gen.GenerateClient(spec, gen.Config{
		OutDir:      clientDir,
		ModulePath:  "test.local/rtmclient",
		PackageName: "rtm",
		GoVersion:   "1.26",
	})
	require.NoError(t, err)

	_, err = gen.GenerateCLI(spec, defaultCLIConfig(cliDir))
	require.NoError(t, err)

	// Stitch the two modules: the CLI module requires the client
	// module by a version that does not exist on the public proxy,
	// so a replace points it at the local client temp dir.
	editCmd := exec.Command("go", "mod", "edit", "-replace=test.local/rtmclient=../client")
	editCmd.Dir = cliDir
	if out, err := editCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod edit failed:\n%s", out)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = cliDir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy in cli failed:\n%s", out)
	}

	// Inject the boundary fixture and its expected-paths data.
	fixture, err := os.ReadFile(filepath.Join("testdata", "cli_boundary_test.go.tmpl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cliDir, "boundary_test.go"), fixture, 0o644))

	paths, err := expectedCLIPaths(spec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cliDir, "expected_paths.txt"), []byte(strings.Join(paths, "\n")+"\n"), 0o644))

	build := exec.Command("go", "build", "./...")
	build.Dir = cliDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... in cli failed:\n%s", out)
	}

	runTest := exec.Command("go", "test", "./...")
	runTest.Dir = cliDir
	out, err := runTest.CombinedOutput()
	if err != nil {
		t.Fatalf("go test in cli failed:\n%s", out)
	}
}

func expectedCLIPaths(spec apispec.Spec) ([]string, error) {
	out := make([]string, 0, len(spec))
	for name := range spec {
		path, err := naming.CLICommand(name)
		if err != nil {
			return nil, err
		}
		out = append(out, strings.Join(path, " "))
	}
	sort.Strings(out)
	return out, nil
}

// TestGeneratedModulePassesBoundarySuite generates a self-contained module
// from the committed spec, injects a handwritten httptest-based test from
// testdata/, and runs `go test` against the result. This is the end-to-end
// verification that the generator's output is usable.
func TestGeneratedModulePassesBoundarySuite(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)

	dir := t.TempDir()
	_, err = gen.GenerateClient(spec, defaultConfig(dir))
	require.NoError(t, err)

	fixture, err := os.ReadFile(filepath.Join("testdata", "boundary_test.go.tmpl"))
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "boundary_test.go"), fixture, 0o644)
	require.NoError(t, err)

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test in generated module failed:\n%s", out)
}
