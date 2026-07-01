package node

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// requestInfo is the request metadata handed to JavaScript when a request
// arrives. It carries everything IncomingMessage needs so the JavaScript side
// never reaches back for request fields.
type requestInfo struct {
	Method        string         `json:"method"`
	URL           string         `json:"url"`
	HTTPVersion   string         `json:"httpVersion"`
	VersionMajor  int            `json:"httpVersionMajor"`
	VersionMinor  int            `json:"httpVersionMinor"`
	Headers       map[string]any `json:"headers"`
	RawHeaders    []string       `json:"rawHeaders"`
	RemoteAddress string         `json:"remoteAddress"`
	RemotePort    int            `json:"remotePort"`
}

// buildRequestInfo snapshots a Go request into the JSON envelope IncomingMessage
// is built from. Header translation follows Node: names lowercase, duplicates
// joined with ", ", and set-cookie kept as an array since it must not be folded.
func buildRequestInfo(r *http.Request) string {
	host, portStr, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	port := 0
	if portStr != "" {
		if p, perr := net.LookupPort("tcp", portStr); perr == nil {
			port = p
		}
	}

	headers := make(map[string]any, len(r.Header))
	raw := make([]string, 0, len(r.Header)*2)
	for name, values := range r.Header {
		lower := strings.ToLower(name)
		if lower == "set-cookie" {
			headers[lower] = append([]string(nil), values...)
		} else {
			headers[lower] = strings.Join(values, ", ")
		}
		for _, v := range values {
			raw = append(raw, name, v)
		}
	}

	info := requestInfo{
		Method:        r.Method,
		URL:           r.URL.RequestURI(),
		HTTPVersion:   strings.TrimPrefix(r.Proto, "HTTP/"),
		VersionMajor:  r.ProtoMajor,
		VersionMinor:  r.ProtoMinor,
		Headers:       headers,
		RawHeaders:    raw,
		RemoteAddress: host,
		RemotePort:    port,
	}
	return jsonString(info)
}

// decodeHeaderPairs reads the [name, value] array the response side sends. A
// malformed payload yields no headers rather than an error, since a header write
// should never crash the response path.
func decodeHeaderPairs(s string) [][2]string {
	var raw [][]string
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil
	}
	out := make([][2]string, 0, len(raw))
	for _, p := range raw {
		if len(p) == 2 {
			out = append(out, [2]string{p[0], p[1]})
		}
	}
	return out
}

// errCode maps a listen error to a Node error code where the cause is clear, so
// EADDRINUSE and EACCES surface with the code Node programs branch on.
func errCode(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "address already in use"):
		return "EADDRINUSE"
	case strings.Contains(msg, "permission denied"):
		return "EACCES"
	case strings.Contains(msg, "cannot assign requested address"):
		return "EADDRNOTAVAIL"
	default:
		return ""
	}
}
