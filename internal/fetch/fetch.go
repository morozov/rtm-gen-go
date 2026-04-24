package fetch

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

// ErrUnexpectedStatus is returned when RTM responds with a non-200
// HTTP status. Callers may distinguish transport failures from
// application failures with errors.Is.
var ErrUnexpectedStatus = errors.New("unexpected HTTP status")

// ErrRTMAPI is the sentinel every RTM application-level failure
// wraps. The fetcher returns *APIError, which satisfies
// errors.Is(err, ErrRTMAPI).
var ErrRTMAPI = errors.New("rtm api error")

// APIError reports a stat=fail response from the RTM API. Code is
// parsed from the wire string; on parse failure it is 0 and the
// raw value is preserved in Message.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("rtm api error %d: %s", e.Code, e.Message)
}

func (e *APIError) Is(target error) bool {
	return target == ErrRTMAPI
}

const (
	defaultBaseURL = "https://api.rememberthemilk.com/services/rest/"
	defaultTimeout = 30 * time.Second
)

// Fetch retrieves the current RTM reflection spec via live HTTP calls
// and returns it as an apispec.Spec. baseURL is empty for production
// use; tests pass an httptest server URL.
func Fetch(ctx context.Context, apiKey, apiSecret, baseURL string) (apispec.Spec, error) {
	raw, err := FetchRaw(ctx, apiKey, apiSecret, baseURL)
	if err != nil {
		return nil, err
	}
	return apispec.Parse(raw)
}

// FetchRaw retrieves the current RTM reflection spec via live HTTP
// calls and returns the assembled JSON bytes — a map of method name to
// per-method reflection payload, the exact shape apispec.Parse expects.
// Callers that only need the parsed spec should use Fetch instead.
func FetchRaw(ctx context.Context, apiKey, apiSecret, baseURL string) ([]byte, error) {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	f := &fetcher{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		http:      &http.Client{Timeout: defaultTimeout},
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
	return raw, nil
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
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)
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
				// RTM's JSON bridge serialises text-only
				// <method>name</method> children as bare
				// strings in the method array. Accept that
				// form directly.
				Method []string `json:"method"`
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
	names = append(names, envelope.Rsp.Methods.Method...)
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
		code, perr := strconv.Atoi(s.Err.Code)
		if perr != nil {
			return &APIError{Code: 0, Message: fmt.Sprintf("%s %s", s.Err.Code, s.Err.Msg)}
		}
		return &APIError{Code: code, Message: s.Err.Msg}
	}
	return &APIError{Code: 0, Message: fmt.Sprintf("rtm returned stat=%q", s.Stat)}
}
