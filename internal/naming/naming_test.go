package naming_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morozov/rtm-gen-go/internal/naming"
)

func TestCLICommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"auth.checkToken", "rtm.auth.checkToken", []string{"auth", "check-token"}},
		{"lists.getList", "rtm.lists.getList", []string{"lists", "get-list"}},
		{"tasks.notes.add", "rtm.tasks.notes.add", []string{"tasks", "notes", "add"}},
		{"single-word method", "rtm.test.echo", []string{"test", "echo"}},
		{"acronym run", "rtm.push.setURL", []string{"push", "set-url"}},
		{"reflection.getMethods", "rtm.reflection.getMethods", []string{"reflection", "get-methods"}},
		{"reflection.getMethodInfo", "rtm.reflection.getMethodInfo", []string{"reflection", "get-method-info"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := naming.CLICommand(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestCLICommand_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"missing rtm prefix", "auth.checkToken"},
		{"empty", ""},
		{"only prefix", "rtm."},
		{"no method segment", "rtm.lists"},
		{"trailing dot", "rtm.lists.getList."},
		{"double dot", "rtm.lists..getList"},
		{"leading dot after prefix", "rtm..lists.getList"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := naming.CLICommand(tc.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, naming.ErrInvalidMethodName)
		})
	}
}

func TestGoMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"checkToken", "rtm.auth.checkToken", "CheckToken"},
		{"getList", "rtm.lists.getList", "GetList"},
		{"nested leaf", "rtm.tasks.notes.add", "Add"},
		{"single-word", "rtm.test.echo", "Echo"},
		{"acronym preserved", "rtm.push.setURL", "SetURL"},
		{"getMethodInfo", "rtm.reflection.getMethodInfo", "GetMethodInfo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := naming.GoMethod(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestGoMethod_Errors(t *testing.T) {
	t.Parallel()

	_, err := naming.GoMethod("invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, naming.ErrInvalidMethodName)
}

func TestGoService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"single segment", "lists", "ListsService"},
		{"auth", "auth", "AuthService"},
		{"nested", "tasks.notes", "TasksNotesService"},
		{"reflection", "reflection", "ReflectionService"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := naming.GoService(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestGoService_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"leading dot", ".lists"},
		{"trailing dot", "lists."},
		{"double dot", "tasks..notes"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := naming.GoService(tc.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, naming.ErrInvalidMethodName)
		})
	}
}

func TestGoLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty", "", ""},
		{"plain", "name", "name"},
		{"two words", "list_id", "listID"},
		{"first-segment initialism stays lowercase", "api_key", "apiKey"},
		{"first segment url", "url", "url"},
		{"trailing url", "custom_url", "customURL"},
		{"multi", "from_list_id", "fromListID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expect, naming.GoLocal(tc.input))
		})
	}
}

func TestGoField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty", "", ""},
		{"plain word", "name", "Name"},
		{"snake to pascal", "list_id", "ListID"},
		{"acronym only", "url", "URL"},
		{"trailing acronym", "custom_url", "CustomURL"},
		{"api prefix", "api_key", "APIKey"},
		{"multi acronym", "list_id_url", "ListIDURL"},
		{"no initialism present", "from_list_id", "FromListID"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expect, naming.GoField(tc.input))
		})
	}
}
