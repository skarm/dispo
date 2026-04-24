`dispo` is a lightweight, high-performance Go package for building safe HTTP `Content-Disposition` headers.

The package correctly handles:
- ASCII and non-ASCII filenames
- Spaces and control characters
- ASCII `filename` fallback with RFC-oriented hardening for `/`, `\`, and `%HH`
- Quoted-string escaping for `"` in `filename`
- UTF-8 `filename*` encoding according to RFC 5987 / RFC 6266

Behavior notes:
- leading and trailing whitespace is trimmed from filenames
- internal ASCII and Unicode whitespace is normalized to ASCII spaces
- ASCII `filename` preserves most printable ASCII characters as-is, except for
  RFC-sensitive fallback rewrites of `/`, `\`, and `%HH` sequences
- non-ASCII runes become `_` in `filename`, while `filename*` carries the exact
  sanitized filename in UTF-8
- invalid UTF-8 byte sequences become `_` in `filename` and U+FFFD in `filename*`
- `filename*` is also emitted when the ASCII fallback had to be rewritten
- disposition types are trimmed; `inline` and `attachment` are normalized to
  lowercase, and other valid HTTP tokens are passed through in lowercase

## API

* `ContentDisposition(dispositionType, name string) string`
  Generates a Content-Disposition header with a normalized disposition type and filename.

* `Attachment(name string) string`
  Shortcut for `attachment`.

* `Inline(name string) string`
  Shortcut for `inline`.
