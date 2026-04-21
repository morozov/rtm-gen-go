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

const fixturePath = "../apispec/testdata/fixture.json"

func defaultConfig(outDir string) gen.Config {
	return gen.Config{
		OutDir:      outDir,
		PackageName: "rtm",
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
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

func TestGenerateEmitsCoreAndPerService(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
	require.NoError(t, err)

	dir := t.TempDir()
	files, err := gen.GenerateClient(spec, defaultConfig(dir))
	require.NoError(t, err)

	names := make(map[string]bool, len(files))
	for _, p := range files {
		names[filepath.Base(p)] = true
	}
	assert.False(t, names["go.mod"], "go.mod MUST NOT be emitted")
	assert.True(t, names["client.go"], "core file must be emitted")
	for _, svc := range []string{"fixture.go", "fixture_sub.go"} {
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
		{"missing OutDir", gen.Config{PackageName: "p"}},
		{"missing PackageName", gen.Config{OutDir: t.TempDir()}},
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
		PackageName:       "commands",
		ClientModulePath:  "test.local/host/internal/rtm",
		ClientPackageName: "rtm",
	}
}

func TestGenerateCLIIsDeterministic(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
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

func TestGenerateCLIEmitsRegister(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
	require.NoError(t, err)

	dir := t.TempDir()
	files, err := gen.GenerateCLI(spec, defaultCLIConfig(dir))
	require.NoError(t, err)

	names := make(map[string]bool, len(files))
	for _, p := range files {
		names[filepath.Base(p)] = true
	}
	assert.True(t, names["register.go"], "register.go must be emitted")
	assert.False(t, names["go.mod"], "go.mod MUST NOT be emitted")
	assert.False(t, names["main.go"], "main.go MUST NOT be emitted")
}

func TestGenerateCLIRequiresConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultCLIConfig(t.TempDir())
	cfg.ClientModulePath = ""
	_, err := gen.GenerateCLI(apispec.Spec{}, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, gen.ErrInvalidConfig)
}

// TestGeneratedHostBuildsAndReachesAllCommands generates both the
// client and commands packages into a single temp host module, wires
// them together with a hand-written go.mod and boundary test file,
// runs `go mod tidy` and `go test`, and asserts the constructed
// command tree covers every method in the synthetic fixture.
func TestGeneratedHostBuildsAndReachesAllCommands(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
	require.NoError(t, err)

	root := t.TempDir()
	clientDir := filepath.Join(root, "internal", "rtm")
	commandsDir := filepath.Join(root, "internal", "commands")
	require.NoError(t, os.MkdirAll(clientDir, 0o755))
	require.NoError(t, os.MkdirAll(commandsDir, 0o755))

	_, err = gen.GenerateClient(spec, gen.Config{
		OutDir:      clientDir,
		PackageName: "rtm",
	})
	require.NoError(t, err)

	_, err = gen.GenerateCLI(spec, gen.CLIConfig{
		OutDir:            commandsDir,
		PackageName:       "commands",
		ClientModulePath:  "test.local/host/internal/rtm",
		ClientPackageName: "rtm",
	})
	require.NoError(t, err)

	writeHostGoMod(t, root, "test.local/host")

	fixture, err := os.ReadFile(filepath.Join("testdata", "cli_boundary_test.go.tmpl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "boundary_test.go"), fixture, 0o644))

	paths, err := expectedCLIPaths(spec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "expected_paths.txt"), []byte(strings.Join(paths, "\n")+"\n"), 0o644))

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed:\n%s", out)
	}

	build := exec.Command("go", "build", "./...")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... failed:\n%s", out)
	}

	runTest := exec.Command("go", "test", "./...")
	runTest.Dir = root
	if out, err := runTest.CombinedOutput(); err != nil {
		t.Fatalf("go test failed:\n%s", out)
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

// TestGeneratedClientPassesBoundarySuite generates the client into a
// standalone module and runs a handwritten httptest-based boundary
// test against it.
func TestGeneratedClientPassesBoundarySuite(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
	require.NoError(t, err)

	root := t.TempDir()
	clientDir := filepath.Join(root, "rtm")
	require.NoError(t, os.MkdirAll(clientDir, 0o755))

	_, err = gen.GenerateClient(spec, gen.Config{
		OutDir:      clientDir,
		PackageName: "rtm",
	})
	require.NoError(t, err)

	writeClientGoMod(t, root, "test.local/client")

	fixture, err := os.ReadFile(filepath.Join("testdata", "boundary_test.go.tmpl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(clientDir, "boundary_test.go"), fixture, 0o644))

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go test failed:\n%s", out)
}

func writeHostGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()
	content := "module " + modulePath + "\n\ngo 1.26\n\nrequire github.com/spf13/cobra v1.10.2\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644))
}

func writeClientGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()
	content := "module " + modulePath + "\n\ngo 1.26\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644))
}
