package fetch

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

const defaultBaseURL = "https://api.rememberthemilk.com/services/rest/"

// Fetch retrieves the current RTM reflection spec via live HTTP calls
// and returns it as an apispec.Spec. baseURL is empty for production
// use; tests pass an httptest server URL.
func Fetch(ctx context.Context, apiKey, apiSecret, baseURL string) (apispec.Spec, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	f := &fetcher{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		http:      http.DefaultClient,
	}

	listBody, err := f.call(ctx, "rtm.reflection.getMethods", nil)
	if err != nil {
		return nil, fmt.Errorf("getMethods: %w", err)
	}
	names, err := parseMethodNames(listBody)
	if err != nil {
		return nil, fmt.Errorf("parse method list: %w", err)
	}

	assembled := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		body, err := f.call(ctx, "rtm.reflection.getMethodInfo", url.Values{"method_name": []string{name}})
		if err != nil {
			return nil, fmt.Errorf("getMethodInfo %s: %w", name, err)
		}
		info, err := parseMethodInfo(body)
		if err != nil {
			return nil, fmt.Errorf("parse methodInfo %s: %w", name, err)
		}
		assembled[name] = info
	}

	raw, err := json.Marshal(assembled)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	return apispec.Parse(raw)
}

type fetcher struct {
	apiKey    string
	apiSecret string
	baseURL   string
	http      *http.Client
}

func (f *fetcher) call(ctx context.Context, method string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("method", method)
	params.Set("format", "json")
	params.Set("api_key", f.apiKey)
	params.Set("api_sig", f.sign(params))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	return body, nil
}

func (f *fetcher) sign(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "api_sig" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(f.apiSecret)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(params.Get(k))
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

type rspStat struct {
	Stat string  `json:"stat"`
	Err  *rtmErr `json:"err,omitempty"`
}

type rtmErr struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

func parseMethodNames(body []byte) ([]string, error) {
	var envelope struct {
		Rsp struct {
			rspStat
			Methods struct {
				Method []struct {
					Name string `json:"name"`
				} `json:"method"`
			} `json:"methods"`
		} `json:"rsp"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if err := checkStat(envelope.Rsp.rspStat); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(envelope.Rsp.Methods.Method))
	for _, m := range envelope.Rsp.Methods.Method {
		names = append(names, m.Name)
	}
	return names, nil
}

func parseMethodInfo(body []byte) (json.RawMessage, error) {
	var envelope struct {
		Rsp struct {
			rspStat
			Method json.RawMessage `json:"method"`
		} `json:"rsp"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if err := checkStat(envelope.Rsp.rspStat); err != nil {
		return nil, err
	}
	return envelope.Rsp.Method, nil
}

func checkStat(s rspStat) error {
	if s.Stat == "ok" {
		return nil
	}
	if s.Err != nil {
		return fmt.Errorf("rtm error %s: %s", s.Err.Code, s.Err.Msg)
	}
	return fmt.Errorf("rtm returned stat=%q", s.Stat)
}
