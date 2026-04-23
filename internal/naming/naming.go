package naming

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalidMethodName is returned when an RTM method name does not
// match the expected "rtm.<service>[.<subservice>...].<method>" shape.
var ErrInvalidMethodName = errors.New("invalid RTM method name")

const rtmPrefix = "rtm."

// CLICommand returns the CLI command path for a fully qualified RTM
// method name.
//
//	"rtm.auth.checkToken" -> []string{"auth", "check-token"}
//	"rtm.tasks.notes.add" -> []string{"tasks", "notes", "add"}
func CLICommand(rtmMethod string) ([]string, error) {
	service, method, err := splitMethod(rtmMethod)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(service)+1)
	out = append(out, service...)
	out = append(out, camelToKebab(method))
	return out, nil
}

// GoMethod returns the PascalCase Go method identifier for an RTM
// method name.
//
//	"rtm.lists.getList" -> "GetList"
//	"rtm.push.setURL"   -> "SetURL"
func GoMethod(rtmMethod string) (string, error) {
	_, method, err := splitMethod(rtmMethod)
	if err != nil {
		return "", err
	}
	return upperFirst(method), nil
}

// GoService returns the Go service type identifier for an RTM service
// path (the dot-separated segments between "rtm." and the method
// segment).
//
//	"lists"       -> "ListsService"
//	"tasks.notes" -> "TasksNotesService"
func GoService(servicePath string) (string, error) {
	if servicePath == "" {
		return "", fmt.Errorf("empty service path: %w", ErrInvalidMethodName)
	}
	var b strings.Builder
	for _, seg := range strings.Split(servicePath, ".") {
		if seg == "" {
			return "", fmt.Errorf("empty segment in %q: %w", servicePath, ErrInvalidMethodName)
		}
		b.WriteString(upperFirst(seg))
	}
	b.WriteString("Service")
	return b.String(), nil
}

// initialisms are snake_case tokens rendered in uppercase when they
// appear as whole segments of a struct field name.
var initialisms = map[string]string{
	"api":  "API",
	"id":   "ID",
	"url":  "URL",
	"json": "JSON",
	"xml":  "XML",
	"html": "HTML",
}

// GoField returns the PascalCase Go identifier for a snake_case RTM
// argument name, recognising common initialisms so "list_id" yields
// "ListID" and "url" yields "URL".
func GoField(argName string) string {
	if argName == "" {
		return ""
	}
	parts := strings.Split(argName, "_")
	var b strings.Builder
	b.Grow(len(argName))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if up, ok := initialisms[strings.ToLower(p)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(upperFirst(p))
	}
	return b.String()
}

// GoLocal returns the lowerCamelCase Go identifier for a snake_case
// RTM argument name. The first segment is always lowercase, even if
// it is an initialism ("api_key" -> "apiKey"); subsequent segments
// honour the initialism table ("list_id" -> "listID").
func GoLocal(argName string) string {
	if argName == "" {
		return ""
	}
	parts := strings.Split(argName, "_")
	var b strings.Builder
	b.Grow(len(argName))
	first := true
	for _, p := range parts {
		if p == "" {
			continue
		}
		if first {
			b.WriteString(strings.ToLower(p))
			first = false
			continue
		}
		if up, ok := initialisms[strings.ToLower(p)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(upperFirst(p))
	}
	return b.String()
}

func splitMethod(rtmMethod string) ([]string, string, error) {
	if !strings.HasPrefix(rtmMethod, rtmPrefix) {
		return nil, "", fmt.Errorf("%q missing %q prefix: %w", rtmMethod, rtmPrefix, ErrInvalidMethodName)
	}
	rest := rtmMethod[len(rtmPrefix):]
	if rest == "" {
		return nil, "", fmt.Errorf("%q has no service or method: %w", rtmMethod, ErrInvalidMethodName)
	}
	parts := strings.Split(rest, ".")
	if len(parts) < 2 {
		return nil, "", fmt.Errorf("%q missing method segment: %w", rtmMethod, ErrInvalidMethodName)
	}
	for i, p := range parts {
		if p == "" {
			return nil, "", fmt.Errorf("%q has empty segment at index %d: %w", rtmMethod, i, ErrInvalidMethodName)
		}
	}
	return parts[:len(parts)-1], parts[len(parts)-1], nil
}

// camelToKebab converts camelCase to kebab-case, treating a run of
// consecutive uppercase letters as a single word so that "setURL"
// yields "set-url" rather than "set-u-r-l".
func camelToKebab(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes) + 4)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next)) {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
