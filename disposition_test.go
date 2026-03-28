package dispo

import (
	"strings"
	"testing"
)

func TestContentDisposition_TypeNormalization(t *testing.T) {
	tests := []struct {
		name      string
		inType    string
		wantDispo string
	}{
		{name: "inline lowercase", inType: "inline", wantDispo: inline},
		{name: "inline uppercase", inType: "INLINE", wantDispo: inline},
		{name: "inline with extra spaces", inType: "  \tInLiNe\n", wantDispo: attachment},
		{name: "attachment explicit", inType: "attachment", wantDispo: attachment},
		{name: "unknown", inType: "form-data", wantDispo: attachment},
		{name: "empty", wantDispo: attachment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ContentDisposition(tt.inType, "file.txt")
			dispo, _, _, _ := parseContentDisposition(t, out)
			if dispo != tt.wantDispo {
				t.Fatalf("disposition mismatch: want=%q got=%q, out=%q", tt.wantDispo, dispo, out)
			}
		})
	}
}

func TestContentDisposition_FilenameCases(t *testing.T) {
	tests := []struct {
		name             string
		in               string
		wantFilename     string
		wantFilenameStar string
		wantHasStar      bool
	}{
		{name: "ascii simple", in: "file.txt", wantFilename: "file.txt"},
		{name: "trim edges keep inner space", in: "  report 2026.txt  ", wantFilename: "report 2026.txt"},
		{name: "unicode letters", in: "Тест.pdf", wantFilename: "____.pdf", wantFilenameStar: "%D0%A2%D0%B5%D1%81%D1%82.pdf", wantHasStar: true},
		{name: "emoji", in: "🔥.txt", wantFilename: "_.txt", wantFilenameStar: "%F0%9F%94%A5.txt", wantHasStar: true},
		{name: "path separators and quote", in: `a/b\c"d.txt`, wantFilename: "a_b_c_d.txt"},
		{name: "controls removed", in: "ab\x00\x01\r\n\tcd.txt", wantFilename: "abcd.txt"},
		{name: "only spaces fallback", in: " \u00A0\u2002 ", wantFilename: defaultFilename},
		{name: "empty fallback", wantFilename: defaultFilename},
		{name: "percent and separators", in: "100%;, ok.txt", wantFilename: "100%;, ok.txt"},
		{name: "crlf injection payload", in: "evil\r\nSet-Cookie: hack=1.txt", wantFilename: "evilSet-Cookie: hack=1.txt"},
		{name: "invalid utf8 bytes", in: string([]byte{0xff, 0xfe, 'a'}), wantFilename: "__a", wantFilenameStar: "%EF%BF%BD%EF%BF%BDa", wantHasStar: true},
		{name: "ascii prefix + non-ascii + ascii after", in: "abc🔥def.txt", wantFilename: "abc_def.txt", wantFilenameStar: "abc%F0%9F%94%A5def.txt", wantHasStar: true},
		{name: "ascii + spaces + non-ascii", in: "a  🔥b.txt", wantFilename: "a  _b.txt", wantFilenameStar: "a%20%20%F0%9F%94%A5b.txt", wantHasStar: true},
		{name: "only non-ascii", in: "🔥🔥🔥.txt", wantFilename: "___.txt", wantFilenameStar: "%F0%9F%94%A5%F0%9F%94%A5%F0%9F%94%A5.txt", wantHasStar: true},
		{name: "ascii + non-ascii + ascii after", in: "a🔥b.txt", wantFilename: "a_b.txt", wantFilenameStar: "a%F0%9F%94%A5b.txt", wantHasStar: true},
		{name: "chinese characters", in: "你好世界.txt", wantFilename: "____.txt", wantFilenameStar: "%E4%BD%A0%E5%A5%BD%E4%B8%96%E7%95%8C.txt", wantHasStar: true},
		{name: "japanese kanji + kana", in: "漢字かな混在.pdf", wantFilename: "______.pdf", wantFilenameStar: "%E6%BC%A2%E5%AD%97%E3%81%8B%E3%81%AA%E6%B7%B7%E5%9C%A8.pdf", wantHasStar: true},
		{name: "arabic letters", in: "مرحبا.txt", wantFilename: "_____.txt", wantFilenameStar: "%D9%85%D8%B1%D8%AD%D8%A8%D8%A7.txt", wantHasStar: true},
		{name: "hebrew letters", in: "שלום.doc", wantFilename: "____.doc", wantFilenameStar: "%D7%A9%D7%9C%D7%95%D7%9D.doc", wantHasStar: true},
		{name: "symbols and math", in: "∑∆√≈.txt", wantFilename: "____.txt", wantFilenameStar: "%E2%88%91%E2%88%86%E2%88%9A%E2%89%88.txt", wantHasStar: true},
		{name: "mixing emojis and CJK", in: "🔥你好.txt", wantFilename: "___.txt", wantFilenameStar: "%F0%9F%94%A5%E4%BD%A0%E5%A5%BD.txt", wantHasStar: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ContentDispositionForAttachment(tt.in)
			dispo, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, out)

			if dispo != attachment {
				t.Fatalf("disposition mismatch: want=%q got=%q", attachment, dispo)
			}
			if filename != tt.wantFilename {
				t.Fatalf("filename mismatch: want=%q got=%q", tt.wantFilename, filename)
			}
			if hasFilenameStar != tt.wantHasStar {
				t.Fatalf("filename* presence mismatch: want=%v got=%v out=%q", tt.wantHasStar, hasFilenameStar, out)
			}
			if filenameStar != tt.wantFilenameStar {
				t.Fatalf("filename* mismatch: want=%q got=%q", tt.wantFilenameStar, filenameStar)
			}
			assertCommonOutputInvariants(t, out, filename, filenameStar, hasFilenameStar)
		})
	}
}

func TestContentDisposition_Wrappers(t *testing.T) {
	name := "my report.txt"

	if got, want := ContentDispositionForAttachment(name), ContentDisposition(attachment, name); got != want {
		t.Fatalf("attachment wrapper mismatch: want=%q got=%q", want, got)
	}

	if got, want := ContentDispositionForInline(name), ContentDisposition(inline, name); got != want {
		t.Fatalf("inline wrapper mismatch: want=%q got=%q", want, got)
	}
}

func TestContentDisposition_NonASCIICtrl(t *testing.T) {
	runes := []rune{'\u0085', '\u2028', '\u2029'}
	for _, r := range runes {
		in := "ab" + string(r) + "cd.txt"
		out := ContentDispositionForAttachment(in)
		_, filename, filenameStar, hasStar := parseContentDisposition(t, out)

		if strings.ContainsAny(filename, "\"\\") {
			t.Fatalf("unsafe chars in filename: %q", filename)
		}
		for i := 0; i < len(filename); i++ {
			b := filename[i]
			if b < 0x20 || b == 0x7f {
				t.Fatalf("control char in filename: %q", filename)
			}
		}

		if hasStar {
			t.Fatalf("unexpected filename* for ASCII-only: %q", out)
		}
		if filenameStar != "" {
			t.Fatalf("unexpected filename*: %q", filenameStar)
		}
	}
}

func TestContentDisposition_InvariantsOnManyInputs(t *testing.T) {
	types := []string{attachment, inline, "INLINE", "", "unknown"}
	inputs := []string{
		"", "a", "file.txt", "   file.txt   ", "你好世界", "🔥🔥🔥",
		"\r\nbad", `quo"te\slash`, "semi;colon,comma", "100%.txt",
		"foo\u00A0bar", string([]byte{0x00, 0x01, 0xff, 0xfe}),
		strings.Repeat("a", 1024), strings.Repeat("🔥", 128),
	}

	for _, dt := range types {
		for _, in := range inputs {
			out1 := ContentDisposition(dt, in)
			dispo, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, out1)
			if dispo != inline && dispo != attachment {
				t.Fatalf("unexpected disposition %q for type=%q input=%q", dispo, dt, in)
			}
			assertCommonOutputInvariants(t, out1, filename, filenameStar, hasFilenameStar)
			out2 := ContentDisposition(dt, in)
			if out1 != out2 {
				t.Fatalf("non-deterministic output for type=%q input=%q: %q != %q", dt, in, out1, out2)
			}
		}
	}
}

func FuzzContentDisposition(f *testing.F) {
	seeds := []struct {
		dt   string
		name string
	}{
		{dt: inline, name: "file.txt"},
		{dt: attachment, name: "Тест.pdf"},
		{dt: "INLINE", name: "🔥 report.txt"},
		{dt: "inline", name: string([]byte{0xff, 0xfe, 0xfd})},
		{dt: " \tinline\n", name: "a/b\\c\"d"},
		{dt: "", name: ""},
	}

	for _, s := range seeds {
		f.Add(s.dt, s.name)
	}

	f.Fuzz(func(t *testing.T, dispositionType, name string) {
		out := ContentDisposition(dispositionType, name)
		dispo, filename, filenameStar, hasFilenameStar := parseContentDisposition(t, out)
		if dispo != inline && dispo != attachment {
			t.Fatalf("unexpected disposition: %q", dispo)
		}
		assertCommonOutputInvariants(t, out, filename, filenameStar, hasFilenameStar)
	})
}

func parseContentDisposition(t *testing.T, out string) (disposition, filename, filenameStar string, hasFilenameStar bool) {
	t.Helper()
	i := strings.Index(out, filenamePrefix)
	j := strings.Index(out, filenameStarPrefix)

	if i < 0 {
		t.Fatalf("invalid Content-Disposition format: %q", out)
	}
	disposition = out[:i]
	start := i + len(filenamePrefix)

	if j >= 0 {
		if j < start {
			t.Fatalf("invalid Content-Disposition format: %q", out)
		}
		filename = out[start:j]
		filenameStar = out[j+len(filenameStarPrefix):]
		return disposition, filename, filenameStar, true
	}

	if !strings.HasSuffix(out, filenameOnlySuffix) || len(out) < start+len(filenameOnlySuffix) {
		t.Fatalf("invalid Content-Disposition format: %q", out)
	}
	filename = out[start : len(out)-len(filenameOnlySuffix)]
	return disposition, filename, "", false
}

func assertCommonOutputInvariants(t *testing.T, out, filename, filenameStar string, hasFilenameStar bool) {
	t.Helper()
	if out == "" {
		t.Fatal("empty output")
	}
	if strings.ContainsAny(out, "\r\n") {
		t.Fatalf("CRLF injection detected: %q", out)
	}
	if filename == "" {
		t.Fatal("empty filename parameter")
	}
	if strings.ContainsAny(filename, "\"\\") {
		t.Fatalf("unsafe quoted filename chars present: %q", filename)
	}
	for i := 0; i < len(filename); i++ {
		b := filename[i]
		if b < 0x20 || b == 0x7f {
			t.Fatalf("control char in filename: %q", filename)
		}
	}
	if hasFilenameStar {
		if filenameStar == "" {
			t.Fatal("empty filename* parameter")
		}
		assertValidRFC8187Value(t, filenameStar)
		return
	}
	if filenameStar != "" {
		t.Fatalf("unexpected filename* value for ascii-only form: %q", filenameStar)
	}
	if strings.Contains(out, "filename*=") {
		t.Fatalf("unexpected filename* token in ascii-only form: %q", out)
	}
}

func assertValidRFC8187Value(t *testing.T, v string) {
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
		if b >= 0x80 || !rfc8187AttrCharTable[b] {
			t.Fatalf("invalid unescaped byte %q in filename*: %q", b, v)
		}
	}
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}
