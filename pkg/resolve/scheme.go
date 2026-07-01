package resolve

import (
	"encoding/base64"
	"net/url"
	"strings"
)

// resolveData resolves a data: URL, decoding its body so the loader can hand the
// bytes straight to the parser. The full data: string is the cache identity.
func resolveData(rest, specifier string) (Resolved, error) {
	meta, payload, ok := strings.Cut(rest, ",")
	if !ok {
		return Resolved{}, &ResolveError{
			Code:      "ERR_INVALID_URL",
			Specifier: specifier,
			Message:   "malformed data: URL",
		}
	}

	mime, base64Encoded := parseDataMeta(meta)

	var body []byte
	if base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return Resolved{}, &ResolveError{
				Code:      "ERR_INVALID_URL",
				Specifier: specifier,
				Message:   "invalid base64 in data: URL",
			}
		}
		body = decoded
	} else {
		unescaped, err := url.PathUnescape(payload)
		if err != nil {
			return Resolved{}, &ResolveError{
				Code:      "ERR_INVALID_URL",
				Specifier: specifier,
				Message:   "invalid percent-encoding in data: URL",
			}
		}
		body = []byte(unescaped)
	}

	format, err := dataFormat(mime, specifier)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Kind:      KindData,
		Format:    format,
		Path:      specifier,
		Specifier: specifier,
		Body:      body,
	}, nil
}

// parseDataMeta splits a data: URL's metadata into a mime type and whether the
// body is base64. It defaults to text/plain per the data: URL spec.
func parseDataMeta(meta string) (mime string, base64Encoded bool) {
	mime = "text/plain"
	first := true
	for meta != "" {
		var part string
		part, meta, _ = strings.Cut(meta, ";")
		if part == "base64" {
			base64Encoded = true
		} else if first && part != "" {
			mime = part
		}
		first = false
	}
	return mime, base64Encoded
}

// dataFormat maps a data: URL mime type to a module format. JSON and JavaScript
// mimes are supported; anything else is an error.
func dataFormat(mime, specifier string) (Format, error) {
	switch {
	case mime == "application/json" || strings.HasSuffix(mime, "+json"):
		return FormatJSON, nil
	case mime == "text/javascript" || mime == "application/javascript" || mime == "text/typescript":
		return FormatESM, nil
	default:
		return FormatUnknown, &ResolveError{
			Code:      "ERR_INVALID_URL",
			Specifier: specifier,
			Message:   "unsupported data: mime type " + mime,
		}
	}
}

// resolveGo validates a go: import path and hands it off. It deliberately does
// not touch the Go module cache or read Go source; pkg/goimport owns that.
func resolveGo(importPath, specifier string) (Resolved, error) {
	if !validGoImportPath(importPath) {
		return Resolved{}, &ResolveError{
			Code:      "ERR_INVALID_MODULE_SPECIFIER",
			Specifier: specifier,
			Message:   "invalid go: import path " + importPath,
		}
	}
	return Resolved{
		Kind:      KindGo,
		Format:    FormatESM,
		Path:      importPath,
		Specifier: specifier,
	}, nil
}

// validGoImportPath is a light check that a go: path looks like a Go import
// path: a dotted host, a slash, and no empty segments or backslashes. The full
// validation lives in pkg/goimport; this only guards the resolver boundary.
func validGoImportPath(p string) bool {
	if p == "" || strings.ContainsAny(p, "\\ \t\n") {
		return false
	}
	slash := strings.IndexByte(p, '/')
	if slash <= 0 {
		return false
	}
	host := p[:slash]
	if !strings.Contains(host, ".") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return false
		}
	}
	return true
}
