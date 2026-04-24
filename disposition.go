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
	"net/textproto"
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

	httpTokenSeparators = `()<>@,;:\"/[]?={} `
	hexUpper            = "0123456789ABCDEF"
	asciiScratchSize    = 256
	encodedScratchSize  = 512
)

var rfc5987AttrCharTable = buildRFC5987AttrCharTable()
var httpTokenCharTable = buildHTTPTokenCharTable()
var asciiSpaceTable = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}
var quotedPairEscapeTable = [256]uint8{'"': 1, '\\': 1}

// ContentDisposition builds a Content-Disposition header value using the
// provided disposition type and filename.
//
// The dispositionType is normalized as follows:
//   - surrounding HTTP whitespace is trimmed
//   - "inline" and "attachment" (case-insensitive) are normalized to lowercase
//   - any other valid HTTP token is returned in lowercase
//   - invalid or empty values fall back to "attachment"
//
// The filename is sanitized and encoded according to RFC 6266 and RFC 5987.
//
// Behavior details:
//   - Control characters are removed.
//   - Leading and trailing whitespace is trimmed.
//   - Internal ASCII and Unicode whitespace is normalized to ASCII spaces.
//   - Invalid UTF-8 byte sequences become '_' in filename and U+FFFD in filename*.
//   - If the resulting filename is empty, only the disposition-type is returned.
//
// ASCII handling (filename parameter):
//   - Always emitted when filename is non-empty.
//   - Encoded as quoted-string.
//   - The fallback avoids `\`, `/`, and `%HH` sequences per RFC 6266 guidance.
//   - `"` is escaped with backslash in the quoted-string form.
//   - Other printable ASCII characters are preserved as-is.
//
// Non-ASCII handling:
//   - Non-ASCII runes are replaced with '_' in the ASCII filename.
//   - The exact sanitized filename is emitted via filename* using RFC 5987
//     ext-value syntax with UTF-8, an empty language tag, and percent-encoded
//     UTF-8 bytes.
//
// Notes:
//   - filename* is included when the ASCII fallback cannot represent the
//     sanitized filename faithfully.
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

	if out, ok := trySimpleASCIIContentDisposition(dispoType, name); ok {
		return out
	}

	var (
		out strings.Builder
		// Tuned scratch capacities keep the common slow path on stack-backed slices
		// and avoid extra heap work for short and medium filenames.
		asciiScratch   [asciiScratchSize]byte
		encodedScratch [encodedScratchSize]byte
	)

	out.Grow(len(dispoType) + len(filenamePrefix) + len(name) + len(filenameSuffix) + len(filenameStarPrefix))
	out.WriteString(dispoType)

	asciiBuf := asciiScratch[:0]
	if len(name)*2 > cap(asciiBuf) {
		asciiBuf = make([]byte, 0, len(name)*2)
	}

	encodedBuf := encodedScratch[:0]
	hasFilenameStar := false
	hasContent := false
	pendingSpaces := 0

	for i := 0; i < len(name); {
		if b := name[i]; b < utf8.RuneSelf {
			idx := i
			i++

			if isASCIIControl(b) {
				continue
			}
			if asciiSpaceTable[b] != 0 {
				if hasContent {
					pendingSpaces++
				}
				continue
			}

			if hasFilenameStar {
				asciiBuf, encodedBuf = appendPendingSpacesToBoth(asciiBuf, encodedBuf, pendingSpaces)
			} else {
				asciiBuf = appendPendingASCIISpaces(asciiBuf, pendingSpaces)
			}

			pendingSpaces = 0
			hasContent = true

			if isASCIIFallbackUnsafeByte(name, idx, b) {
				if !hasFilenameStar {
					out.Grow(len(filenameStarPrefix) + len(name)*3)
					if len(name)*3 > cap(encodedBuf) {
						encodedBuf = make([]byte, 0, len(name)*3)
					}
					encodedBuf = appendRFC5987EncodedASCII(encodedBuf, name[:idx])
					hasFilenameStar = true
				}

				asciiBuf = append(asciiBuf, '_')
				encodedBuf = appendRFC5987EncodedByte(encodedBuf, b)
				continue
			}

			asciiBuf = appendQuotedStringByte(asciiBuf, b)

			if hasFilenameStar {
				encodedBuf = appendRFC5987EncodedByte(encodedBuf, b)
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

		if !hasFilenameStar {
			out.Grow(len(filenameStarPrefix) + len(name)*3)
			if len(name)*3 > cap(encodedBuf) {
				encodedBuf = make([]byte, 0, len(name)*3)
			}
			encodedBuf = appendRFC5987EncodedASCII(encodedBuf, name[:i-size])
			for range pendingSpaces {
				encodedBuf = appendRFC5987EncodedByte(encodedBuf, ' ')
			}
			asciiBuf = appendPendingASCIISpaces(asciiBuf, pendingSpaces)
			hasFilenameStar = true
		} else {
			asciiBuf, encodedBuf = appendPendingSpacesToBoth(asciiBuf, encodedBuf, pendingSpaces)
		}

		pendingSpaces = 0
		hasContent = true
		asciiBuf = append(asciiBuf, '_')
		encodedBuf = appendRFC5987EncodedRune(encodedBuf, r)
	}

	if !hasContent {
		return dispoType
	}

	if !hasFilenameStar && rewriteRFC6266PercentHazards(asciiBuf) {
		out.Grow(len(filenameStarPrefix) + len(name)*3)
		if len(name)*3 > cap(encodedBuf) {
			encodedBuf = make([]byte, 0, len(name)*3)
		}
		encodedBuf = appendRFC5987EncodedASCII(encodedBuf, name)
		hasFilenameStar = true
	}

	out.WriteString(filenamePrefix)
	out.Write(asciiBuf)
	out.WriteString(filenameSuffix)

	if hasFilenameStar {
		out.WriteString(filenameStarPrefix)
		out.Write(encodedBuf)
	}

	return out.String()
}

func trySimpleASCIIContentDisposition(dispoType, name string) (string, bool) {
	if name == "" {
		return dispoType, true
	}

	for i := 0; i < len(name); i++ {
		b := name[i]

		if b == ' ' {
			if i == 0 || i == len(name)-1 {
				return "", false
			}
			continue
		}

		switch {
		case b >= utf8.RuneSelf:
			return "", false
		case isASCIIControl(b):
			return "", false
		case b == '"' || b == '\\' || b == '/':
			return "", false
		case b == '%' && i+2 < len(name) && isHex(name[i+1]) && isHex(name[i+2]):
			return "", false
		}
	}

	return dispoType + filenamePrefix + name + filenameSuffix, true
}

func normalizeDispositionType(v string) string {
	v = textproto.TrimString(v)
	if !isHTTPToken(v) {
		return attachment
	}

	switch {
	case strings.EqualFold(v, inline):
		return inline
	case strings.EqualFold(v, attachment):
		return attachment
	default:
		return strings.ToLower(v)
	}
}

func isASCIIControl(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}

func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		if !httpTokenCharTable[s[i]] {
			return false
		}
	}

	return true
}

func appendPendingASCIISpaces(asciiBuf []byte, pendingSpaces int) []byte {
	for range pendingSpaces {
		asciiBuf = append(asciiBuf, ' ')
	}
	return asciiBuf
}

func appendPendingSpacesToBoth(asciiBuf, encodedBuf []byte, pendingSpaces int) ([]byte, []byte) {
	for range pendingSpaces {
		asciiBuf = append(asciiBuf, ' ')
		encodedBuf = appendRFC5987EncodedByte(encodedBuf, ' ')
	}
	return asciiBuf, encodedBuf
}

func appendQuotedStringByte(asciiBuf []byte, b byte) []byte {
	if quotedPairEscapeTable[b] != 0 {
		return append(asciiBuf, '\\', b)
	}
	return append(asciiBuf, b)
}

func isASCIIFallbackUnsafeByte(name string, i int, b byte) bool {
	if b == '/' || b == '\\' {
		return true
	}
	return b == '%' && i+2 < len(name) && isHex(name[i+1]) && isHex(name[i+2])
}

func rewriteRFC6266PercentHazards(asciiBuf []byte) bool {
	replaced := false

	for i := 0; i+2 < len(asciiBuf); i++ {
		if asciiBuf[i] == '%' && isHex(asciiBuf[i+1]) && isHex(asciiBuf[i+2]) {
			asciiBuf[i] = '_'
			replaced = true
		}
	}

	return replaced
}

func appendRFC5987EncodedASCII(buf []byte, s string) []byte {
	written := false
	pendingSpaces := 0

	for i := 0; i < len(s); i++ {
		b := s[i]

		if isASCIIControl(b) {
			continue
		}

		if asciiSpaceTable[b] != 0 {
			if written {
				pendingSpaces++
			}
			continue
		}

		for range pendingSpaces {
			buf = appendRFC5987EncodedByte(buf, ' ')
		}

		pendingSpaces = 0
		written = true
		buf = appendRFC5987EncodedByte(buf, b)
	}

	return buf
}

func appendRFC5987EncodedRune(encodedBuf []byte, r rune) []byte {
	var runeBuf [utf8.UTFMax]byte

	n := utf8.EncodeRune(runeBuf[:], r)
	for i := 0; i < n; i++ {
		encodedBuf = appendRFC5987EncodedByte(encodedBuf, runeBuf[i])
	}

	return encodedBuf
}

func appendRFC5987EncodedByte(buf []byte, b byte) []byte {
	if rfc5987AttrCharTable[b] {
		return append(buf, b)
	}
	return append(buf, '%', hexUpper[b>>4], hexUpper[b&0x0f])
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

func buildHTTPTokenCharTable() [256]bool {
	var table [256]bool

	for b := byte(0x21); b < 0x7f; b++ {
		table[b] = true
	}

	for i := 0; i < len(httpTokenSeparators); i++ {
		table[httpTokenSeparators[i]] = false
	}

	return table
}
