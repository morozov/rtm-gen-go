package apispec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

const expectedMethodCount = 64

// committedDumpPath points at the repository-root api.json. The loader
// test reads it directly; there is no duplicated copy under testdata.
var committedDumpPath = filepath.Join("..", "..", "api.json")

func TestLoad_CommittedDump(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(committedDumpPath)
	require.NoError(t, err)
	require.Len(t, spec, expectedMethodCount, "committed api.json should list all RTM methods")

	t.Run("every entry has a matching name", func(t *testing.T) {
		t.Parallel()
		for key, m := range spec {
			assert.Equal(t, key, m.Name, "map key must match method name")
		}
	})

	t.Run("every entry has at least one argument and one error", func(t *testing.T) {
		t.Parallel()
		for _, m := range spec {
			assert.NotEmpty(t, m.Arguments, "method %q should declare arguments", m.Name)
			assert.NotEmpty(t, m.Errors, "method %q should declare errors", m.Name)
		}
	})

	t.Run("checkToken is parsed correctly", func(t *testing.T) {
		t.Parallel()
		m, ok := spec["rtm.auth.checkToken"]
		require.True(t, ok)
		assert.Equal(t, "rtm.auth.checkToken", m.Name)
		assert.False(t, m.NeedsLogin)
		assert.False(t, m.NeedsSigning)
		assert.False(t, m.NeedsTimeline)
		assert.Equal(t, "0", m.RequiredPerms)
		assert.NotEmpty(t, m.Description)
		assert.NotEmpty(t, m.Response)
		require.Len(t, m.Arguments, 2)
		assert.Equal(t, "api_key", m.Arguments[0].Name)
		assert.False(t, m.Arguments[0].Optional)
		assert.Equal(t, "auth_token", m.Arguments[1].Name)
	})
}

func TestParse_InvalidBool(t *testing.T) {
	t.Parallel()

	input := []byte(`{
		"rtm.example": {
			"name": "rtm.example",
			"needslogin": "maybe",
			"needssigning": "0",
			"requiredperms": "0",
			"description": "",
			"response": "",
			"arguments": {"argument": []},
			"errors": {"error": []}
		}
	}`)

	_, err := apispec.Parse(input)
	require.Error(t, err)
	assert.ErrorIs(t, err, apispec.ErrInvalidBool)
}

func TestParse_Malformed(t *testing.T) {
	t.Parallel()

	_, err := apispec.Parse([]byte(`not json`))
	require.Error(t, err)
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := apispec.Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
