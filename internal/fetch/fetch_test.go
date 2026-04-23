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
	}
}

func TestFetchRawReturnsMapShape(t *testing.T) {
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
			writeJSON(w, map[string]any{
				"rsp": map[string]any{
					"stat":    "ok",
					"methods": map[string]any{"method": toMethodList(methods)},
				},
			})
		case "rtm.reflection.getMethodInfo":
			if body, ok := findMethod(methods, q.Get("method_name")); ok {
				_, _ = w.Write([]byte(body))
				return
			}
			http.Error(w, "unknown", http.StatusNotFound)
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	raw, err := fetch.FetchRaw(context.Background(), "key", "secret", srv.URL+"/rest/")
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	for _, mf := range methods {
		_, ok := m[mf.name]
		assert.True(t, ok, "expected method %q in raw map", mf.name)
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
	require.ErrorIs(t, err, fetch.ErrRTMAPI)
	var apiErr *fetch.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 100, apiErr.Code)
	assert.Equal(t, "Invalid API Key", apiErr.Message)
}

func TestFetchSurfacesUnexpectedHTTPStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := fetch.Fetch(context.Background(), "key", "secret", srv.URL+"/rest/")
	require.Error(t, err)
	assert.ErrorIs(t, err, fetch.ErrUnexpectedStatus)
}

func TestFetchRejectsMalformedMethodList(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rsp":{"stat":"ok","methods":`))
	}))
	t.Cleanup(srv.Close)

	_, err := fetch.Fetch(context.Background(), "key", "secret", srv.URL+"/rest/")
	require.Error(t, err)
	assert.NotErrorIs(t, err, fetch.ErrRTMAPI, "JSON parse failure should not look like an RTM API error")
	assert.NotErrorIs(t, err, fetch.ErrUnexpectedStatus, "JSON parse failure should not look like an HTTP failure")
}

func TestFetchHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"rsp":{"stat":"ok"}}`))
		_ = r
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetch.Fetch(ctx, "key", "secret", srv.URL+"/rest/")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
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
