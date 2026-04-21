//go:build livefetch

package fetch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/morozov/rtm-gen-go/internal/fetch"
)

type methodFixture struct {
	name string
	body string
}

func TestFetchAssemblesSpec(t *testing.T) {
	t.Parallel()

	methods := []methodFixture{
		{
			name: "rtm.alpha.ping",
			body: `{"rsp":{"stat":"ok","method":{"name":"rtm.alpha.ping","needslogin":"0","needssigning":"0","requiredperms":"0","description":"Synthetic ping.","response":"<rsp stat=\"ok\"/>","arguments":{"argument":[{"name":"api_key","optional":"0","$t":"API key."}]},"errors":{"error":[{"code":"1","message":"err","$t":"synthetic"}]}}}}`,
		},
		{
			name: "rtm.alpha.beta.nested",
			body: `{"rsp":{"stat":"ok","method":{"name":"rtm.alpha.beta.nested","needslogin":"0","needssigning":"0","requiredperms":"0","description":"Nested.","response":"<rsp stat=\"ok\"/>","arguments":{"argument":[{"name":"api_key","optional":"0","$t":"API key."}]},"errors":{"error":[{"code":"1","message":"err","$t":"synthetic"}]}}}}`,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("method") {
		case "rtm.reflection.getMethods":
			list := map[string]any{
				"rsp": map[string]any{
					"stat": "ok",
					"methods": map[string]any{
						"method": toMethodList(methods),
					},
				},
			}
			writeJSON(w, list)
		case "rtm.reflection.getMethodInfo":
			name := q.Get("method_name")
			if body, ok := findMethod(methods, name); ok {
				_, _ = w.Write([]byte(body))
				return
			}
			http.Error(w, fmt.Sprintf("unknown method %q", name), http.StatusNotFound)
		default:
			http.Error(w, fmt.Sprintf("unexpected method %q", q.Get("method")), http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	spec, err := fetch.Fetch(context.Background(), "key", "secret", srv.URL+"/rest/")
	require.NoError(t, err)
	require.Len(t, spec, len(methods))

	for _, m := range methods {
		entry, ok := spec[m.name]
		require.True(t, ok, "method %q should be present", m.name)
		assert.Equal(t, m.name, entry.Name)
		assert.NotEmpty(t, entry.Arguments)
		assert.NotEmpty(t, entry.Errors)
	}
}

func TestFetchSurfacesRTMError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rsp":{"stat":"fail","err":{"code":"100","msg":"Invalid API Key"}}}`))
	}))
	t.Cleanup(srv.Close)

	_, err := fetch.Fetch(context.Background(), "key", "secret", srv.URL+"/rest/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid API Key")
}

func toMethodList(methods []methodFixture) []map[string]string {
	out := make([]map[string]string, 0, len(methods))
	for _, m := range methods {
		out = append(out, map[string]string{"name": m.name})
	}
	return out
}

func findMethod(methods []methodFixture, name string) (string, bool) {
	for _, m := range methods {
		if m.name == name {
			return m.body, true
		}
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
