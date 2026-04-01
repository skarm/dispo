package dispo_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/skarm/dispo"
)

const (
	testFilenamePrefix     = `; filename="`
	testFilenameStarPrefix = `; filename*=UTF-8''`
)

func TestContentDisposition_FilenameCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ascii simple", in: "file.txt", want: `attachment; filename="file.txt"`},
		{name: "trim edges keep inner space", in: "  report 2026.txt  ", want: `attachment; filename="report 2026.txt"`},
		{name: "unicode letters", in: "Тест.pdf", want: `attachment; filename="____.pdf"; filename*=UTF-8''%D0%A2%D0%B5%D1%81%D1%82.pdf`},
		{name: "emoji", in: "🔥.txt", want: `attachment; filename="_.txt"; filename*=UTF-8''%F0%9F%94%A5.txt`},
		{name: "path separators and quote", in: `a/b\c"d.txt`, want: `attachment; filename="a/b\\c\"d.txt"`},
		{name: "controls removed", in: "ab\x00\x01\r\n\tcd.txt", want: `attachment; filename="abcd.txt"`},
		{name: "only spaces fallback", in: " \u00A0\u2002 ", want: `attachment`},
		{name: "empty fallback", want: `attachment`},
		{name: "percent kept", in: "100%.txt", want: `attachment; filename="100%.txt"`},
		{name: "percent and others", in: "100%;, ok.txt", want: `attachment; filename="100%;, ok.txt"`},
		{name: "crlf injection payload", in: "evil\r\nSet-Cookie: hack=1.txt", want: `attachment; filename="evilSet-Cookie: hack=1.txt"`},
		{name: "invalid utf8 bytes", in: string([]byte{0xff, 0xfe, 'a'}), want: `attachment; filename="__a"; filename*=UTF-8''%EF%BF%BD%EF%BF%BDa`},
		{name: "ascii prefix + non-ascii + ascii after", in: "abc🔥def.txt", want: `attachment; filename="abc_def.txt"; filename*=UTF-8''abc%F0%9F%94%A5def.txt`},
		{name: "ascii + spaces + non-ascii", in: "a  🔥b.txt", want: `attachment; filename="a  _b.txt"; filename*=UTF-8''a%20%20%F0%9F%94%A5b.txt`},
		{name: "only non-ascii", in: "🔥🔥🔥.txt", want: `attachment; filename="___.txt"; filename*=UTF-8''%F0%9F%94%A5%F0%9F%94%A5%F0%9F%94%A5.txt`},
		{name: "ascii + non-ascii + ascii after", in: "a🔥b.txt", want: `attachment; filename="a_b.txt"; filename*=UTF-8''a%F0%9F%94%A5b.txt`},
		{name: "chinese characters", in: "你好世界.txt", want: `attachment; filename="____.txt"; filename*=UTF-8''%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.txt`},
		{name: "japanese kanji + kana", in: "漢字かな混在.pdf", want: `attachment; filename="______.pdf"; filename*=UTF-8''%E6%BC%A2%E5%AD%97%E3%81%8B%E3%81%AA%E6%B7%B7%E5%9C%A8.pdf`},
		{name: "arabic letters", in: "مرحبا.txt", want: `attachment; filename="_____.txt"; filename*=UTF-8''%D9%85%D8%B1%D8%AD%D8%A8%D8%A7.txt`},
		{name: "hebrew letters", in: "שלום.doc", want: `attachment; filename="____.doc"; filename*=UTF-8''%D7%A9%D7%9C%D7%95%D7%9D.doc`},
		{name: "symbols and math", in: "∑∆√≈.txt", want: `attachment; filename="____.txt"; filename*=UTF-8''%E2%88%91%E2%88%86%E2%88%9A%E2%89%88.txt`},
		{name: "mixing emojis and CJK", in: "🔥你好.txt", want: `attachment; filename="___.txt"; filename*=UTF-8''%F0%9F%94%A5%E4%BD%A0%E5%A5%BD.txt`},
		{name: "slash only", in: "a/b", want: `attachment; filename="a/b"`},
		{name: "backslash only", in: `a\b`, want: `attachment; filename="a\\b"`},
		{name: "quote only", in: `a"b`, want: `attachment; filename="a\"b"`},
		{name: "percent only", in: "a%b", want: `attachment; filename="a%b"`},
		{name: "apostrophe", in: "it's.txt", want: `attachment; filename="it's.txt"`},
		{name: "asterisk", in: "file*.txt", want: `attachment; filename="file*.txt"`},
		{name: "percent + unicode", in: "100%🔥.txt", want: `attachment; filename="100%_.txt"; filename*=UTF-8''100%25%F0%9F%94%A5.txt`},
		{name: "brackets", in: "file[1].txt", want: `attachment; filename="file[1].txt"`},
		{name: "angle brackets", in: "file<1>.txt", want: `attachment; filename="file<1>.txt"`},
		{name: "equals", in: "a=b.txt", want: `attachment; filename="a=b.txt"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispo.Attachment(tt.in)

			if got != tt.want {
				t.Fatalf("mismatch:\nwant=%q\ngot =%q", tt.want, got)
			}

			disposition, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, got)
			if disposition != "attachment" {
				t.Fatalf("disposition mismatch: %q", disposition)
			}

			assertCommonOutputInvariants(t, got, disposition, filename, filenameStar, hasFilenameStar)
		})
	}
}

func TestContentDisposition_TypeNormalization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"inline lowercase", "inline", `inline; filename="file.txt"`},
		{"inline uppercase", "INLINE", `inline; filename="file.txt"`},
		{"inline with extra spaces", "  \tInLiNe\n", `attachment; filename="file.txt"`},
		{"attachment explicit", "attachment", `attachment; filename="file.txt"`},
		{"unknown", "form-data", `attachment; filename="file.txt"`},
		{"empty", "", `attachment; filename="file.txt"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dispo.ContentDisposition(tt.in, "file.txt")
			if got != tt.want {
				t.Fatalf("mismatch:\nwant=%q\ngot =%q", tt.want, got)
			}
		})
	}
}

func TestContentDisposition_Wrappers(t *testing.T) {
	name := "my report.txt"
	wantAttachment := `attachment; filename="my report.txt"`
	wantInline := `inline; filename="my report.txt"`

	if got := dispo.Attachment(name); got != wantAttachment {
		t.Fatalf("attachment mismatch:\nwant=%q\ngot =%q", wantAttachment, got)
	}

	if got := dispo.Inline(name); got != wantInline {
		t.Fatalf("inline mismatch:\nwant=%q\ngot =%q", wantInline, got)
	}
}

func TestContentDisposition_NonASCIICtrl(t *testing.T) {
	runes := []rune{'\u0085', '\u2028', '\u2029'}

	for _, r := range runes {
		in := "ab" + string(r) + "cd.txt"

		got := dispo.Attachment(in)
		if got == "" {
			t.Fatal("empty output")
		}

		_, filename, filenameStar, hasStar := parseContentDisposition(t, got)

		if !hasQuotedStringSafety(filename) {
			t.Fatalf("unsafe chars in filename: %q", filename)
		}

		if hasStar {
			t.Fatalf("unexpected filename*: %q", got)
		}
		if filenameStar != "" {
			t.Fatalf("unexpected filename*: %q", filenameStar)
		}
	}
}

func TestContentDisposition_InvariantsOnManyInputs(t *testing.T) {
	types := []string{"attachment", "inline", "INLINE", "", "unknown"}
	inputs := []string{
		"", "a", "file.txt", "   file.txt   ", "你好世界", "🔥🔥🔥",
		"\r\nbad", `quo"te\slash`, "semi;colon,comma", "100%.txt",
		"foo\u00A0bar", string([]byte{0x00, 0x01, 0xff, 0xfe}),
		strings.Repeat("a", 1024), strings.Repeat("🔥", 128),
	}

	for _, dt := range types {
		for _, in := range inputs {
			out1 := dispo.ContentDisposition(dt, in)
			out2 := dispo.ContentDisposition(dt, in)

			if out1 == "" {
				t.Fatalf("empty output for type=%q input=%q", dt, in)
			}
			if out1 != out2 {
				t.Fatalf("non-deterministic:\n%q\n!=\n%q", out1, out2)
			}

			disposition, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, out1)

			if disposition != "inline" && disposition != "attachment" {
				t.Fatalf("bad disposition %q", disposition)
			}

			assertCommonOutputInvariants(t, out1, disposition, filename, filenameStar, hasFilenameStar)
		}
	}
}

func TestContentDisposition_UTF8RoundTrip(t *testing.T) {
	cases := []string{
		"Тест.pdf",
		"🔥.txt",
		"abc🔥def.txt",
		"你好世界.txt",
		"漢字かな混在.pdf",
	}

	for _, original := range cases {
		t.Run(original, func(t *testing.T) {
			header := dispo.Attachment(original)

			start := strings.Index(header, testFilenameStarPrefix)
			if start < 0 {
				t.Fatalf("filename* not found in header: %q", header)
			}
			encoded := header[start+len(testFilenameStarPrefix):]

			decoded, err := url.PathUnescape(encoded)
			if err != nil {
				t.Fatalf("failed to percent-decode filename*: %v", err)
			}

			if decoded != original {
				t.Fatalf("UTF-8 roundtrip failed: want=%q got=%q", original, decoded)
			}
		})
	}
}

func FuzzContentDisposition(f *testing.F) {
	seeds := []struct {
		dt   string
		name string
	}{
		{dt: "inline", name: "file.txt"},
		{dt: "attachment", name: "Тест.pdf"},
		{dt: "INLINE", name: "🔥 report.txt"},
		{dt: "inline", name: string([]byte{0xff, 0xfe, 0xfd})},
		{dt: " \tinline\n", name: `a/b\c"d`},
		{dt: "", name: ""},
	}

	for _, s := range seeds {
		f.Add(s.dt, s.name)
	}

	f.Fuzz(func(t *testing.T, dispositionType, name string) {
		out1 := dispo.ContentDisposition(dispositionType, name)
		out2 := dispo.ContentDisposition(dispositionType, name)

		if out1 == "" {
			t.Fatal("empty output")
		}
		if out1 != out2 {
			t.Fatalf("non-deterministic:\n%q\n!=\n%q", out1, out2)
		}

		dispo, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, out1)

		if dispo != "inline" && dispo != "attachment" {
			t.Fatalf("unexpected disposition: %q", dispo)
		}

		assertCommonOutputInvariants(t, out1, dispo, filename, filenameStar, hasFilenameStar)
	})
}

func parseContentDisposition(t *testing.T, out string) (disposition, filename, filenameStar string, hasFilenameStar bool) {
	t.Helper()

	i := strings.Index(out, testFilenamePrefix)
	if i < 0 {
		return out, "", "", false
	}

	disposition = out[:i]
	start := i + len(testFilenamePrefix)

	closeIdx, ok := scanQuotedString(out, start)
	if !ok {
		t.Fatalf("invalid Content-Disposition format: %q", out)
	}

	filename = out[start:closeIdx]

	afterFilename := closeIdx + 1
	j := strings.Index(out[afterFilename:], testFilenameStarPrefix)
	if j >= 0 {
		starStart := afterFilename + j + len(testFilenameStarPrefix)
		filenameStar = out[starStart:]
		return disposition, filename, filenameStar, true
	}

	return disposition, filename, "", false
}

func scanQuotedString(s string, start int) (closeIdx int, ok bool) {
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) {
				return 0, false
			}
			i++
		case '"':
			return i, true
		}
	}
	return 0, false
}

func assertCommonOutputInvariants(t *testing.T, out, disposition, filename, filenameStar string, hasFilenameStar bool) {
	t.Helper()

	if out == "" {
		t.Fatal("empty output")
	}
	if strings.ContainsAny(out, "\r\n") {
		t.Fatalf("CRLF injection detected: %q", out)
	}

	if filename == "" {
		if out != disposition {
			t.Fatalf("expected disposition-only output, got %q", out)
		}
		if hasFilenameStar {
			t.Fatalf("unexpected filename* for disposition-only output: %q", out)
		}
		if filenameStar != "" {
			t.Fatalf("unexpected filename* value for disposition-only output: %q", filenameStar)
		}
		return
	}

	if !hasQuotedStringSafety(filename) {
		t.Fatalf("unsafe quoted filename content: %q", filename)
	}

	if hasFilenameStar {
		if filenameStar == "" {
			t.Fatal("empty filename* parameter")
		}
		assertValidRFC5987Value(t, filenameStar)
		return
	}

	if filenameStar != "" {
		t.Fatalf("unexpected filename* value for ascii-only form: %q", filenameStar)
	}
	if strings.Contains(out, testFilenameStarPrefix) {
		t.Fatalf("unexpected filename* token in ascii-only form: %q", out)
	}
}

func hasQuotedStringSafety(v string) bool {
	for i := 0; i < len(v); i++ {
		b := v[i]
		if b < 0x20 || b == 0x7f {
			return false
		}
		if b == '\\' {
			if i+1 >= len(v) {
				return false
			}
			next := v[i+1]
			if next != '\\' && next != '"' {
				return false
			}
			i++
			continue
		}
		if b == '"' {
			return false
		}
	}
	return true
}

func assertValidRFC5987Value(t *testing.T, v string) {
	t.Helper()

	for i := 0; i < len(v); i++ {
		b := v[i]
		if b == '%' {
			if i+2 >= len(v) || !isHex(v[i+1]) || !isHex(v[i+2]) {
				t.Fatalf("broken percent encoding in filename*: %q", v)
			}
			i += 2
			continue
		}
		if b >= 0x80 || !isRFC5987AttrChar(b) {
			t.Fatalf("invalid unescaped byte %q in filename*: %q", b, v)
		}
	}
}

func isRFC5987AttrChar(b byte) bool {
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b >= '0' && b <= '9' {
		return true
	}

	switch b {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}
