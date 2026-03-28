package dispo

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	inline     = "inline"
	attachment = "attachment"

	defaultFilename = "file"

	filenamePrefix     = `; filename="`
	filenameStarPrefix = `"; filename*=UTF-8''`
	filenameOnlySuffix = `"`

	hexUpper = "0123456789ABCDEF"
)

var rfc8187AttrCharTable = buildRFC8187AttrCharTable()

// ContentDisposition returns a properly formatted Content-Disposition header
// value for the given disposition type and filename. The dispositionType is
// normalized to either "inline" or "attachment" according to RFC 6266.
// The filename is sanitized and encoded if necessary to handle non-ASCII
// characters.
func ContentDisposition(dispositionType, name string) string {
	return contentDisposition(normalizeDispositionType(dispositionType), name)
}

// ContentDispositionForAttachment returns a Content-Disposition header value
// explicitly set as "attachment" for the given filename. Non-ASCII characters
// in the filename are percent-encoded following RFC 8187.
func ContentDispositionForAttachment(name string) string {
	return contentDisposition(attachment, name)
}

// ContentDispositionForInline returns a Content-Disposition header value
// explicitly set as "inline" for the given filename. Non-ASCII characters
// in the filename are percent-encoded following RFC 8187.
func ContentDispositionForInline(name string) string {
	return contentDisposition(inline, name)
}

func contentDisposition(dispoType, name string) string {
	asciiFilename, encodedFilename, hasStar := buildFilenameParts(name)

	var out strings.Builder

	if hasStar {
		out.Grow(len(dispoType) + len(filenamePrefix) + len(asciiFilename) + len(filenameStarPrefix) + len(encodedFilename))
	} else {
		out.Grow(len(dispoType) + len(filenamePrefix) + len(asciiFilename) + len(filenameOnlySuffix))
	}

	out.WriteString(dispoType)
	out.WriteString(filenamePrefix)
	out.WriteString(asciiFilename)

	if hasStar {
		out.WriteString(filenameStarPrefix)
		out.WriteString(encodedFilename)
	} else {
		out.WriteString(filenameOnlySuffix)
	}

	return out.String()
}

func normalizeDispositionType(v string) string {
	if strings.EqualFold(v, inline) {
		return inline
	}
	return attachment
}

func buildFilenameParts(name string) (asciiFilename, encodedFilename string, hasStar bool) {
	if name == "" {
		return defaultFilename, "", false
	}

	var ascii, encoded strings.Builder

	ascii.Grow(len(name))
	encoded.Grow(len(name) * 3)

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
			if !written {
				continue
			}
			pendingSpaces++
			continue
		}

		if pendingSpaces > 0 {
			for j := 0; j < pendingSpaces; j++ {
				ascii.WriteByte(' ')
				if hasNonASCII {
					appendEncodedByte(&encoded, ' ')
				}
			}
			pendingSpaces = 0
		}

		written = true

		if r <= 0x7e {
			b := byte(r)
			if b == '/' || b == '\\' || b == '"' {
				b = '_'
			}
			ascii.WriteByte(b)
			if hasNonASCII {
				appendEncodedByte(&encoded, b)
			}
			continue
		}

		if !hasNonASCII {
			hasNonASCII = true
			for j := 0; j < ascii.Len(); j++ {
				appendEncodedByte(&encoded, ascii.String()[j])
			}
		}

		ascii.WriteByte('_')
		appendEncodedRune(&encoded, r)
	}

	if !written {
		return defaultFilename, "", false
	}

	if hasNonASCII {
		return ascii.String(), encoded.String(), true
	}

	return ascii.String(), "", false
}

func appendEncodedRune(sb *strings.Builder, r rune) {
	if r <= utf8.RuneSelf-1 {
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
	if rfc8187AttrCharTable[b] {
		sb.WriteByte(b)
		return
	}

	sb.WriteByte('%')
	sb.WriteByte(hexUpper[b>>4])
	sb.WriteByte(hexUpper[b&0x0f])
}

func buildRFC8187AttrCharTable() [256]bool {
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
