`dispo` is a lightweight, high-performance Go package for building safe HTTP `Content-Disposition` headers.

The package correctly handles:
- ASCII and non-ASCII filenames
- Spaces and control characters
- Unsafe characters `/ \ "` replaced with `_`
- UTF-8 `filename*` encoding according to RFC 8187

## API

* `ContentDisposition(dispositionType, name string) string`
  Generates a Content-Disposition header with the specified type (`inline` or `attachment`) and filename.

* `Attachment(name string) string`
  Shortcut for `attachment`.

* `Inline(name string) string`
  Shortcut for `inline`.