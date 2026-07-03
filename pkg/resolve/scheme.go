package resolve

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/tamnd/bento/pkg/goimport"
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

// resolveGo parses a go: import through pkg/goimport, which owns the specifier
// grammar, and hands it off. It deliberately does not touch the Go module cache or
// read Go source; that is pkg/goimport's job at build time. The parser accepts a
// standard-library path with no dotted host and splits an inline @version pin off
// the import path, both of which the resolver's earlier hand-rolled check got
// wrong. The parsed version rides along on GoVersion so the build can reconcile it
// against go.mod (section 4.3) rather than silently dropping the pin.
func resolveGo(rest, specifier string) (Resolved, error) {
	spec, err := goimport.ParseBody(rest)
	if err != nil {
		return Resolved{}, &ResolveError{
			Code:      "ERR_INVALID_MODULE_SPECIFIER",
			Specifier: specifier,
			Message:   err.Error(),
		}
	}
	return Resolved{
		Kind:      KindGo,
		Format:    FormatESM,
		Path:      spec.ImportPath,
		GoVersion: spec.Version,
		Specifier: specifier,
	}, nil
}
