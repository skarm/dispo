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
var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}
var quotedStringEscape = [256]uint8{'"': 1, '\\': 1}

// ContentDisposition builds a Content-Disposition header value using the
// provided disposition type and filename.
//
// The dispositionType is normalized to either "inline" or "attachment":
//   - "inline" (case-insensitive) -> "inline"
//   - any other value -> "attachment"
//
// The filename is sanitized and encoded according to RFC 6266 and RFC 5987.
//
// Behavior details:
//   - Control characters are removed.
//   - Leading and trailing whitespace is trimmed.
//   - Internal ASCII and Unicode whitespace is normalized to ASCII spaces.
//   - Invalid UTF-8 sequences are replaced with the Unicode replacement character '_'.
//   - If the resulting filename is empty, only the disposition-type is returned.
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

	var out, encodedBuf strings.Builder

	out.Grow(len(dispoType) + len(filenamePrefix) + len(name) + len(filenameSuffix) + len(filenameStarPrefix))
	out.WriteString(dispoType)

	asciiBuf := make([]byte, 0, len(name))
	hasNonASCII := false
	hasContent := false
	pendingSpaces := 0

	for i := 0; i < len(name); {
		if b := name[i]; b < utf8.RuneSelf {
			i++

			if isASCIIControl(b) {
				continue
			}
			if asciiSpace[b] != 0 {
				if hasContent {
					pendingSpaces++
				}
				continue
			}

			if hasNonASCII {
				asciiBuf = flushPendingSpacesBoth(asciiBuf, &encodedBuf, pendingSpaces)
			} else {
				asciiBuf = flushPendingSpacesASCII(asciiBuf, pendingSpaces)
			}

			pendingSpaces = 0
			hasContent = true
			asciiBuf = appendQuotedASCIIByte(asciiBuf, b)

			if hasNonASCII {
				appendEncodedByte(&encodedBuf, b)
			}

			continue
		}

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

		if !hasNonASCII {
			encodedBuf.Grow(len(name) * 3)
			appendEncodedSanitizedASCIIPrefix(&encodedBuf, name[:i-size])
			for range pendingSpaces {
				appendEncodedByte(&encodedBuf, ' ')
			}
			asciiBuf = flushPendingSpacesASCII(asciiBuf, pendingSpaces)
			hasNonASCII = true
		} else {
			asciiBuf = flushPendingSpacesBoth(asciiBuf, &encodedBuf, pendingSpaces)
		}

		pendingSpaces = 0
		hasContent = true
		asciiBuf = append(asciiBuf, '_')
		appendEncodedRune(&encodedBuf, r)
	}

	if !hasContent {
		return dispoType
	}

	out.WriteString(filenamePrefix)
	out.Write(asciiBuf)
	out.WriteString(filenameSuffix)

	if hasNonASCII {
		out.WriteString(filenameStarPrefix)
		out.WriteString(encodedBuf.String())
	}

	return out.String()
}

func normalizeDispositionType(v string) string {
	if strings.EqualFold(v, inline) {
		return inline
	}
	return attachment
}

func isASCIIControl(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func flushPendingSpacesASCII(asciiBuf []byte, pendingSpaces int) []byte {
	for range pendingSpaces {
		asciiBuf = append(asciiBuf, ' ')
	}
	return asciiBuf
}

func flushPendingSpacesBoth(asciiBuf []byte, encoded *strings.Builder, pendingSpaces int) []byte {
	for range pendingSpaces {
		asciiBuf = append(asciiBuf, ' ')
		appendEncodedByte(encoded, ' ')
	}
	return asciiBuf
}

func appendQuotedASCIIByte(asciiBuf []byte, b byte) []byte {
	if quotedStringEscape[b] != 0 {
		return append(asciiBuf, '\\', b)
	}
	return append(asciiBuf, b)
}

func appendEncodedSanitizedASCIIPrefix(sb *strings.Builder, s string) {
	written := false
	pendingSpaces := 0

	for i := 0; i < len(s); i++ {
		b := s[i]

		if isASCIIControl(b) {
			continue
		}

		if asciiSpace[b] != 0 {
			if written {
				pendingSpaces++
			}
			continue
		}

		for range pendingSpaces {
			appendEncodedByte(sb, ' ')
		}

		pendingSpaces = 0
		written = true
		appendEncodedByte(sb, b)
	}
}

func appendEncodedRune(sb *strings.Builder, r rune) {
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
