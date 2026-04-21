package apispec

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads an api.json dump from the given path and returns the
// parsed spec.
func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses the contents of an api.json dump.
func Parse(data []byte) (Spec, error) {
	var raw map[string]rawMethod
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	out := make(Spec, len(raw))
	for name, rm := range raw {
		m, err := rm.toMethod()
		if err != nil {
			return nil, fmt.Errorf("method %q: %w", name, err)
		}
		out[name] = m
	}
	return out, nil
}

type rawMethod struct {
	Name          string  `json:"name"`
	NeedsLogin    string  `json:"needslogin"`
	NeedsSigning  string  `json:"needssigning"`
	NeedsTimeline string  `json:"needstimeline"`
	RequiredPerms string  `json:"requiredperms"`
	Description   string  `json:"description"`
	Response      string  `json:"response"`
	Arguments     rawArgs `json:"arguments"`
	Errors        rawErrs `json:"errors"`
}

type rawArgs struct {
	Argument []rawArgument `json:"argument"`
}

type rawArgument struct {
	Name        string `json:"name"`
	Optional    string `json:"optional"`
	Description string `json:"$t"`
}

type rawErrs struct {
	Error []rawError `json:"error"`
}

type rawError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Description string `json:"$t"`
}

func (rm rawMethod) toMethod() (Method, error) {
	login, err := parseBool(rm.NeedsLogin)
	if err != nil {
		return Method{}, fmt.Errorf("needslogin: %w", err)
	}
	sign, err := parseBool(rm.NeedsSigning)
	if err != nil {
		return Method{}, fmt.Errorf("needssigning: %w", err)
	}
	timeline := false
	if rm.NeedsTimeline != "" {
		timeline, err = parseBool(rm.NeedsTimeline)
		if err != nil {
			return Method{}, fmt.Errorf("needstimeline: %w", err)
		}
	}
	args := make([]Argument, 0, len(rm.Arguments.Argument))
	for i, ra := range rm.Arguments.Argument {
		opt, err := parseBool(ra.Optional)
		if err != nil {
			return Method{}, fmt.Errorf("argument %d %q: %w", i, ra.Name, err)
		}
		args = append(args, Argument{
			Name:        ra.Name,
			Optional:    opt,
			Description: ra.Description,
		})
	}
	errs := make([]MethodError, 0, len(rm.Errors.Error))
	for _, re := range rm.Errors.Error {
		errs = append(errs, MethodError(re))
	}
	return Method{
		Name:          rm.Name,
		NeedsLogin:    login,
		NeedsSigning:  sign,
		NeedsTimeline: timeline,
		RequiredPerms: rm.RequiredPerms,
		Description:   rm.Description,
		Response:      rm.Response,
		Arguments:     args,
		Errors:        errs,
	}, nil
}

func parseBool(s string) (bool, error) {
	switch s {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("%q: %w", s, ErrInvalidBool)
	}
}
