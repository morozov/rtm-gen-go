package gen

import (
	"strings"
	"testing"
)

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

func TestParseSampleXMLStripsRspEnvelope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
	}{
		{"empty rsp", `<rsp stat="ok"/>`},
		{"rsp with stat and child", `<rsp stat="ok"><auth><user id="1"/></auth></rsp>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, err := parseSampleXML(tc.in)
			if err != nil {
				t.Fatalf("parseSampleXML: %v", err)
			}
			for _, c := range root.Children {
				if c.Name == "rsp" {
					t.Fatalf("rsp envelope leaked into root.Children")
				}
			}
			for _, a := range root.Attrs {
				if a == "stat" {
					t.Fatalf("stat attribute leaked into root.Attrs")
				}
			}
		})
	}
}

func TestParseSampleXMLReportsParserError(t *testing.T) {
	t.Parallel()

	_, err := parseSampleXML(`<rsp stat="ok"><auth></rsp>`)
	if err == nil {
		t.Fatal("expected error for malformed XML")
	}
	if !strings.Contains(err.Error(), "parse sample XML") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeDescriptionWithoutBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "boilerplate anchor drops entirely",
			in:   `Adds a contact. <a href="https://www.rememberthemilk.com/services/api/methods/rtm.contacts.add.rtm">here</a> for more details.`,
			want: "Adds a contact.",
		},
		{
			name: "non-boilerplate anchor keeps inner text",
			in:   `Parses a date via <a href="https://www.rememberthemilk.com/services/api/methods/rtm.time.parse.rtm">rtm.time.parse</a>.`,
			want: "Parses a date via rtm.time.parse.",
		},
		{
			name: "html entities decoded after stripping tags",
			in:   `Use the &lt;list&gt; element with <code>id</code>.`,
			want: "Use the <list> element with id.",
		},
		{
			name: "whitespace and stray space-before-comma collapse",
			in:   "Foo  \n bar <code>baz</code>, qux.",
			want: "Foo bar baz, qux.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDescription(tc.in, nil); got != tc.want {
				t.Fatalf("normalizeDescription(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeDescriptionWithBuilderNumbersFootnotes(t *testing.T) {
	t.Parallel()

	b := newRefBuilder()
	in := `First <a href="https://example.com/a">link A</a>, again <a href="https://example.com/a">A</a>, then <a href="https://example.com/b">B</a>.`
	got := normalizeDescription(in, b)

	const want = `First link A[^1], again A[^1], then B[^2].`
	if got != want {
		t.Fatalf("normalizeDescription = %q, want %q", got, want)
	}
	refs := b.references()
	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %d", len(refs))
	}
	if refs[0].N != 1 || refs[1].N != 2 {
		t.Fatalf("references not numbered 1,2 in order: %+v", refs)
	}
}

func TestNormalizeDescriptionBoilerplateUsesFootnoteWhenBuilderPresent(t *testing.T) {
	t.Parallel()

	b := newRefBuilder()
	in := `Adds a contact. <a href="https://example.com/x">here</a> for more details.`
	got := normalizeDescription(in, b)
	const want = "Adds a contact.[^1]"
	if got != want {
		t.Fatalf("normalizeDescription = %q, want %q", got, want)
	}
	refs := b.references()
	if len(refs) != 1 || refs[0].N != 1 {
		t.Fatalf("expected single ref [^1], got %+v", refs)
	}
}
