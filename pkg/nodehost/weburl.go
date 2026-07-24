package nodehost

import (
	"encoding/json"
	"net/url"
	"strings"
)

// URLParseJSON resolves input against an optional base and returns the parsed
// WHATWG components as a JSON string. base is the empty string when there is no
// base. On failure it returns {"ok":false} so the URL constructor can throw a
// TypeError, matching the WHATWG contract that an invalid URL is a hard error
// rather than a null result. Parsing a URL correctly (percent encoding, IDNA
// hosts, default ports, base resolution) is a lot of surface to reimplement in
// JavaScript, and Go's net/url already does it, so the URL class keeps its state
// as fields the JavaScript side reads and only calls back here to (re)parse.
func URLParseJSON(input, base string) string {
	input = strings.TrimSpace(input)

	var u *url.URL
	var err error
	if base != "" {
		b, berr := url.Parse(base)
		if berr != nil || !b.IsAbs() {
			return urlFail()
		}
		u, err = b.Parse(input)
	} else {
		u, err = url.Parse(input)
	}
	if err != nil || !u.IsAbs() {
		return urlFail()
	}

	return urlJSON(urlComponents(u))
}

type urlResult struct {
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

func urlFail() string { return urlJSON(urlResult{OK: false}) }

// urlJSON marshals a result envelope to a string for return across the bridge.
func urlJSON(r urlResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false}`
	}
	return string(b)
}

// urlComponents projects a parsed URL onto the WHATWG property set. The leading
// punctuation (":" on protocol, "?" on search, "#" on hash) is included so the
// JavaScript getters return exactly what a browser returns.
func urlComponents(u *url.URL) urlResult {
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

	return urlResult{
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
