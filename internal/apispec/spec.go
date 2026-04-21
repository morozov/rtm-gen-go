package apispec

import "errors"

// ErrInvalidBool is returned when a string-boolean field contains a
// value other than "0" or "1".
var ErrInvalidBool = errors.New("invalid boolean value")

// Spec is the parsed form of an api.json dump. Keys are full RTM method
// names such as "rtm.auth.checkToken".
type Spec map[string]Method

// Method describes a single RTM method as returned by
// rtm.reflection.getMethodInfo.
type Method struct {
	Name          string
	NeedsLogin    bool
	NeedsSigning  bool
	NeedsTimeline bool
	RequiredPerms string
	Description   string
	Response      string
	Arguments     []Argument
	Errors        []MethodError
}

// Argument describes one argument accepted by a Method.
type Argument struct {
	Name        string
	Optional    bool
	Description string
}

// MethodError describes one error a Method may return.
type MethodError struct {
	Code        string
	Message     string
	Description string
}
