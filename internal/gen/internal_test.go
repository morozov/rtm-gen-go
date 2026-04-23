package gen

import "testing"

func TestArgTypeInfoCoversEveryArgType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in                          argType
		wantGo, wantFlag, wantWire  string
	}{
		{argTypeBool, "bool", "BoolVar", "rtmFormatBool"},
		{argTypeInt, "int64", "Int64Var", "rtmFormatInt"},
		{argTypeStringSlice, "[]string", "StringSliceVar", "rtmJoinStringSlice"},
		{argTypeString, "string", "StringVar", ""},
	}
	for _, tc := range cases {
		gotGo, gotFlag, gotWire := argTypeInfo(tc.in)
		if gotGo != tc.wantGo || gotFlag != tc.wantFlag || gotWire != tc.wantWire {
			t.Errorf("argTypeInfo(%v) = (%q, %q, %q), want (%q, %q, %q)",
				tc.in, gotGo, gotFlag, gotWire, tc.wantGo, tc.wantFlag, tc.wantWire)
		}
	}
}

func TestDefaultLiteralForMatchesZeroValue(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"bool":     "false",
		"int64":    "0",
		"[]string": "nil",
		"string":   `""`,
		"unknown":  `""`, // default case
	}
	for in, want := range cases {
		if got := defaultLiteralFor(in); got != want {
			t.Errorf("defaultLiteralFor(%q) = %q, want %q", in, got, want)
		}
	}
}
