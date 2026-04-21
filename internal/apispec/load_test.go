package apispec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

var fixturePath = filepath.Join("testdata", "fixture.json")

func TestLoadFixture(t *testing.T) {
	t.Parallel()

	spec, err := apispec.Load(fixturePath)
	require.NoError(t, err)
	require.Len(t, spec, 4, "fixture should list all synthetic methods")

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

	t.Run("mixed method flags and arguments parse correctly", func(t *testing.T) {
		t.Parallel()
		m, ok := spec["rtm.fixture.mixed"]
		require.True(t, ok)
		assert.True(t, m.NeedsLogin)
		assert.True(t, m.NeedsSigning)
		assert.True(t, m.NeedsTimeline)
		require.Len(t, m.Arguments, 5)
		assert.Equal(t, "api_key", m.Arguments[0].Name)
		assert.Equal(t, "list_id", m.Arguments[3].Name)
		assert.False(t, m.Arguments[3].Optional)
		assert.Equal(t, "note_text", m.Arguments[4].Name)
		assert.True(t, m.Arguments[4].Optional)
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
