// Package dispo provides helpers to build RFC-compliant Content-Disposition
// header values (RFC 6266) with support for internationalized filenames
// using RFC 5987 encoding.
//
// The implementation focuses on:
//   - security (removal of control characters, CRLF injection prevention)
//   - interoperability (ASCII fallback + filename*)
//   - performance (minimal allocations)
//   - strict adherence to RFC 6266 and RFC 5987 where applicable
package dispo

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	inline     = "inline"
	attachment = "attachment"

	filenamePrefix     = `; filename="`
	filenameStarPrefix = `; filename*=UTF-8''`
	filenameSuffix     = `"`

	hexUpper = "0123456789ABCDEF"
)

var rfc5987AttrCharTable = buildRFC5987AttrCharTable()

// ContentDisposition builds a Content-Disposition header value using the
// provided disposition type and filename.
//
// The dispositionType is normalized to either "inline" or "attachment":
//   - "inline" (case-insensitive) → "inline"
//   - any other value → "attachment"
//
// The filename is sanitized and encoded according to RFC 6266 and RFC 5987.
//
// Behavior details:
//   - Control characters (unicode.IsControl) are removed.
//   - Leading and trailing whitespace is trimmed.
//   - Internal whitespace is preserved.
//   - Invalid UTF-8 sequences are replaced with the Unicode replacement character '_'.
//   - If the resulting filename is empty, only the disposition-type is returned
//     (no filename parameter is emitted).
//
// ASCII handling (filename parameter):
//   - Always emitted when filename is non-empty.
//   - Encoded as quoted-string.
//   - Characters `\` and `"` are escaped with backslash as required by RFC 2616.
//   - Other ASCII characters (including '/', '%', ';', ',') are preserved as-is.
//
// Non-ASCII handling:
//   - Non-ASCII runes are replaced with '_' in the ASCII filename.
//   - The original UTF-8 filename is emitted via filename* using RFC 5987:
//     filename*=UTF-8”<percent-encoded UTF-8>
//
// Example:
//
//	ContentDisposition("attachment", "тест.txt")
//	  → `attachment; filename="____.txt"; filename*=UTF-8''%D1%82%D0%B5%D1%81%D1%82.txt`
//
// Notes:
//   - filename* is only included if non-ASCII characters are present.
//   - Spaces in filename* are encoded as %20.
//   - The function does not enforce filename length limits.
func ContentDisposition(dispositionType, name string) string {
	return contentDisposition(normalizeDispositionType(dispositionType), name)
}

// Attachment is a shorthand for ContentDisposition("attachment", name).
//
// See ContentDisposition for full behavior description.
func Attachment(name string) string {
	return contentDisposition(attachment, name)
}

// Inline is a shorthand for ContentDisposition("inline", name).
//
// See ContentDisposition for full behavior description.
func Inline(name string) string {
	return contentDisposition(inline, name)
}

func contentDisposition(dispoType, name string) string {
	if name == "" {
		return dispoType
	}

	var out, encoded strings.Builder

	out.Grow(len(dispoType) + len(filenamePrefix) + len(name) + len(filenameSuffix) + len(filenameStarPrefix))
	out.WriteString(dispoType)

	asciiBuf := make([]byte, 0, len(name))
	hasNonASCII := false
	written := false
	pendingSpaces := 0

	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		i += size

		if unicode.IsControl(r) {
			continue
		}

		if unicode.IsSpace(r) {
			if written {
				pendingSpaces++
			}
			continue
		}

		if pendingSpaces > 0 {
			for j := 0; j < pendingSpaces; j++ {
				asciiBuf = append(asciiBuf, ' ')
				if hasNonASCII {
					appendEncodedByte(&encoded, ' ')
				}
			}
			pendingSpaces = 0
		}

		written = true

		if r < utf8.RuneSelf {
			b := byte(r)

			if b == '"' || b == '\\' {
				asciiBuf = append(asciiBuf, '\\', b)
			} else {
				asciiBuf = append(asciiBuf, b)
			}

			if hasNonASCII {
				appendEncodedByte(&encoded, b)
			}
			continue
		}

		if !hasNonASCII {
			hasNonASCII = true
			encoded.Grow(len(name) * 3)

			for _, b := range asciiBuf {
				appendEncodedByte(&encoded, b)
			}
		}

		asciiBuf = append(asciiBuf, '_')
		appendEncodedRune(&encoded, r)
	}

	if !written {
		return dispoType
	}

	out.WriteString(filenamePrefix)
	out.Write(asciiBuf)
	out.WriteString(filenameSuffix)

	if hasNonASCII {
		out.WriteString(filenameStarPrefix)
		out.WriteString(encoded.String())
	}

	return out.String()
}

func normalizeDispositionType(v string) string {
	if strings.EqualFold(v, inline) {
		return inline
	}
	return attachment
}

func appendEncodedRune(sb *strings.Builder, r rune) {
	if r < utf8.RuneSelf {
		appendEncodedByte(sb, byte(r))
		return
	}

	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	for i := 0; i < n; i++ {
		appendEncodedByte(sb, buf[i])
	}
}

func appendEncodedByte(sb *strings.Builder, b byte) {
	if rfc5987AttrCharTable[b] {
		sb.WriteByte(b)
		return
	}

	sb.WriteByte('%')
	sb.WriteByte(hexUpper[b>>4])
	sb.WriteByte(hexUpper[b&0x0f])
}

func buildRFC5987AttrCharTable() [256]bool {
	var table [256]bool

	for b := byte('a'); b <= byte('z'); b++ {
		table[b] = true
	}
	for b := byte('A'); b <= byte('Z'); b++ {
		table[b] = true
	}
	for b := byte('0'); b <= byte('9'); b++ {
		table[b] = true
	}
	for i := 0; i < len("!#$&+-.^_`|~"); i++ {
		table["!#$&+-.^_`|~"[i]] = true
	}

	return table
}
