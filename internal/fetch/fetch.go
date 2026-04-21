//go:build livefetch

package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/morozov/rtm-client-go"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

// Fetch retrieves the current RTM reflection spec using the generated
// client and returns it as an apispec.Spec. The baseURL argument is
// empty for production use; tests pass an httptest server URL.
func Fetch(ctx context.Context, apiKey, apiSecret, baseURL string) (apispec.Spec, error) {
	c := rtm.NewClient(apiKey, apiSecret, "")
	if baseURL != "" {
		c.BaseURL = baseURL
	}
	c.HTTP = &http.Client{}

	listBody, err := c.Reflection.GetMethods(ctx)
	if err != nil {
		return nil, fmt.Errorf("getMethods: %w", err)
	}
	names, err := parseMethodNames(listBody)
	if err != nil {
		return nil, fmt.Errorf("parse method list: %w", err)
	}

	assembled := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		body, err := c.Reflection.GetMethodInfo(ctx, rtm.ReflectionGetMethodInfoParams{
			MethodName: name,
		})
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
