package dispo_test

import (
	"mime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/skarm/dispo"
)

func BenchmarkContentDispositionCurrentASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "report-2026-final.txt", dispo.ContentDisposition)
}

func BenchmarkContentDispositionStdlibASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "report-2026-final.txt", contentDispositionStdlib)
}

func BenchmarkContentDispositionReplaceASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "report-2026-final.txt", contentDispositionReplace)
}

func BenchmarkContentDispositionCurrentEscapedASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", `a/b\c"d.txt`, dispo.ContentDisposition)
}

func BenchmarkContentDispositionStdlibEscapedASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", `a/b\c"d.txt`, contentDispositionStdlib)
}

func BenchmarkContentDispositionReplaceEscapedASCII(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", `a/b\c"d.txt`, contentDispositionReplace)
}

func BenchmarkContentDispositionCurrentUnicode(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "Тест🔥final.pdf", dispo.ContentDisposition)
}

func BenchmarkContentDispositionStdlibUnicode(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "Тест🔥final.pdf", contentDispositionStdlib)
}

func BenchmarkContentDispositionReplaceUnicode(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", "Тест🔥final.pdf", contentDispositionReplace)
}

func BenchmarkContentDispositionCurrentLongMixed(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", ` quarterly 你好 report "final" \ ревизия 🔥.xlsx `, dispo.ContentDisposition)
}

func BenchmarkContentDispositionStdlibLongMixed(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", ` quarterly 你好 report "final" \ ревизия 🔥.xlsx `, contentDispositionStdlib)
}

func BenchmarkContentDispositionReplaceLongMixed(b *testing.B) {
	benchmarkContentDisposition(b, "attachment", ` quarterly 你好 report "final" \ ревизия 🔥.xlsx `, contentDispositionReplace)
}

func benchmarkContentDisposition(b *testing.B, dispositionType, name string, fn func(string, string) string) {
	b.ReportAllocs()
	for b.Loop() {
		_ = fn(dispositionType, name)
	}
}

// contentDispositionStdlib is a stdlib-only baseline that aims to stay close
// to dispo.ContentDisposition semantics without using the optimized fast path.
func contentDispositionStdlib(dispositionType, name string) string {
	dispoType := normalizeDispositionTypeStdlib(dispositionType)
	sanitized := sanitizeFilenameStdlib(name)
	if sanitized == "" {
		return dispoType
	}

	asciiName, hasNonASCII := asciiFilenameFallbackStdlib(sanitized)
	main := mime.FormatMediaType(dispoType, map[string]string{"filename": asciiName})
	if main == "" {
		return dispoType
	}
	if !hasNonASCII {
		return main
	}

	var out strings.Builder
	out.Grow(len(main) + len(testFilenameStarPrefix) + len(sanitized)*3)
	out.WriteString(main)
	out.WriteString(testFilenameStarPrefix)
	appendRFC5987EncodedStringStdlib(&out, sanitized)
	return out.String()
}

// contentDispositionReplace is a string-heavy stdlib baseline built mostly from
// Trim/Replace operations and mime.FormatMediaType.
func contentDispositionReplace(dispositionType, name string) string {
	dispoType := normalizeDispositionTypeStdlib(dispositionType)
	sanitized := sanitizeFilenameReplace(name)
	if sanitized == "" {
		return dispoType
	}

	asciiName, hasNonASCII := asciiFilenameFallbackReplace(sanitized)
	main := mime.FormatMediaType(dispoType, map[string]string{"filename": asciiName})
	if main == "" {
		return dispoType
	}
	if !hasNonASCII {
		return main
	}

	var out strings.Builder
	out.Grow(len(main) + len(testFilenameStarPrefix) + len(sanitized)*3)
	out.WriteString(main)
	out.WriteString(testFilenameStarPrefix)
	appendRFC5987EncodedStringStdlib(&out, sanitized)
	return out.String()
}

func normalizeDispositionTypeStdlib(v string) string {
	if strings.EqualFold(v, "inline") {
		return "inline"
	}
	return "attachment"
}

func sanitizeFilenameReplace(name string) string {
	if name == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"\x00", "",
		"\x01", "",
		"\x02", "",
		"\x03", "",
		"\x04", "",
		"\x05", "",
		"\x06", "",
		"\x07", "",
		"\x08", "",
		"\x09", " ",
		"\x0a", " ",
		"\x0b", " ",
		"\x0c", " ",
		"\x0d", " ",
		"\x0e", "",
		"\x0f", "",
		"\x10", "",
		"\x11", "",
		"\x12", "",
		"\x13", "",
		"\x14", "",
		"\x15", "",
		"\x16", "",
		"\x17", "",
		"\x18", "",
		"\x19", "",
		"\x1a", "",
		"\x1b", "",
		"\x1c", "",
		"\x1d", "",
		"\x1e", "",
		"\x1f", "",
		"\x7f", "",
	)

	s := replacer.Replace(name)
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return -1
		case unicode.IsSpace(r):
			return ' '
		default:
			return r
		}
	}, s)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func sanitizeFilenameStdlib(name string) string {
	if name == "" {
		return ""
	}

	var out strings.Builder
	out.Grow(len(name))

	hasContent := false
	pendingSpaces := 0

	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		i += size

		if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) {
			if hasContent {
				pendingSpaces++
			}
			continue
		}

		for range pendingSpaces {
			out.WriteByte(' ')
		}
		pendingSpaces = 0
		hasContent = true
		out.WriteRune(r)
	}

	if !hasContent {
		return ""
	}

	return out.String()
}

func asciiFilenameFallbackStdlib(name string) (string, bool) {
	var out strings.Builder
	out.Grow(len(name))

	hasNonASCII := false
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		i += size

		if r < utf8.RuneSelf {
			if r == '"' || r == '\\' {
				out.WriteByte('\\')
			}
			out.WriteByte(byte(r))
			continue
		}

		hasNonASCII = true
		out.WriteByte('_')
	}

	return out.String(), hasNonASCII
}

func asciiFilenameFallbackReplace(name string) (string, bool) {
	hasNonASCII := false
	var out strings.Builder
	out.Grow(len(name) * 2)

	for _, r := range name {
		if r < utf8.RuneSelf {
			if r == '"' || r == '\\' {
				out.WriteByte('\\')
			}
			out.WriteByte(byte(r))
			continue
		}

		hasNonASCII = true
		out.WriteByte('_')
	}

	return out.String(), hasNonASCII
}

func appendRFC5987EncodedStringStdlib(out *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		appendRFC5987EncodedByteStdlib(out, s[i])
	}
}

func appendRFC5987EncodedByteStdlib(out *strings.Builder, b byte) {
	switch {
	case 'a' <= b && b <= 'z':
		out.WriteByte(b)
	case 'A' <= b && b <= 'Z':
		out.WriteByte(b)
	case '0' <= b && b <= '9':
		out.WriteByte(b)
	case strings.ContainsRune("!#$&+-.^_`|~", rune(b)):
		out.WriteByte(b)
	default:
		const hexUpper = "0123456789ABCDEF"
		out.WriteByte('%')
		out.WriteByte(hexUpper[b>>4])
		out.WriteByte(hexUpper[b&0x0f])
	}
}
