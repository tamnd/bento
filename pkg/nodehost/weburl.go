package nodehost

import (
	"encoding/json"
	"net/url"
	"strings"
)

// URLParse resolves input against an optional base and returns the parsed WHATWG
// components. base is the empty string when there is no base. ok is false for an
// input that is not a valid absolute URL, which both callers turn into the
// TypeError the WHATWG contract requires: an invalid URL is a hard error, not a
// null result.
//
// Parsing a URL correctly (percent encoding, IDNA hosts, default ports, base
// resolution) is a lot of surface to reimplement, and Go's net/url already does
// it, so this is the one implementation. The interpreter reaches it through
// URLParseJSON below; the AOT path calls it directly, since it has no bridge to
// cross and no reason to pay for a marshal and a parse to move a struct across a
// package boundary.
func URLParse(input, base string) (URLComponents, bool) {
	input = strings.TrimSpace(input)

	var u *url.URL
	var err error
	if base != "" {
		b, berr := url.Parse(base)
		if berr != nil || !b.IsAbs() {
			return URLComponents{}, false
		}
		u, err = b.Parse(input)
	} else {
		u, err = url.Parse(input)
	}
	if err != nil || !u.IsAbs() {
		return URLComponents{}, false
	}

	return urlComponents(u), true
}

// URLParseJSON is URLParse marshaled for the interpreter's host bridge, which
// moves strings and not structs. On failure it returns {"ok":false}.
func URLParseJSON(input, base string) string {
	parts, ok := URLParse(input, base)
	if !ok {
		return urlFail()
	}
	return urlJSON(parts)
}

// URLComponents is a parsed URL projected onto the WHATWG property set. The JSON
// tags are the interpreter's wire format, so they are part of the contract with
// pkg/node/js/url.js.
type URLComponents struct {
	OK       bool   `json:"ok"`
	Href     string `json:"href"`
	Protocol string `json:"protocol"`
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Hostname string `json:"hostname"`
	Port     string `json:"port"`
	Pathname string `json:"pathname"`
	Search   string `json:"search"`
	Hash     string `json:"hash"`
	Origin   string `json:"origin"`
}

func urlFail() string { return urlJSON(URLComponents{OK: false}) }

// urlJSON marshals a result envelope to a string for return across the bridge.
func urlJSON(r URLComponents) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false}`
	}
	return string(b)
}

// urlComponents projects a parsed URL onto the WHATWG property set. The leading
// punctuation (":" on protocol, "?" on search, "#" on hash) is included so the
// JavaScript getters return exactly what a browser returns.
func urlComponents(u *url.URL) URLComponents {
	pathname := u.EscapedPath()
	if pathname == "" && u.Host != "" {
		pathname = "/"
	}

	search := ""
	if u.ForceQuery || u.RawQuery != "" {
		search = "?" + u.RawQuery
	}

	hash := ""
	if u.Fragment != "" {
		hash = "#" + u.EscapedFragment()
	}

	username, password := "", ""
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	origin := "null"
	if isSpecialScheme(u.Scheme) {
		origin = u.Scheme + "://" + u.Host
	}

	return URLComponents{
		OK:       true,
		Href:     u.String(),
		Protocol: u.Scheme + ":",
		Username: username,
		Password: password,
		Host:     u.Host,
		Hostname: u.Hostname(),
		Port:     u.Port(),
		Pathname: pathname,
		Search:   search,
		Hash:     hash,
		Origin:   origin,
	}
}

// isSpecialScheme reports whether a scheme has a tuple origin per the WHATWG URL
// spec. Only these expose a non-null origin; others (file, data, custom) do not.
func isSpecialScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "ws", "wss", "ftp":
		return true
	}
	return false
}
