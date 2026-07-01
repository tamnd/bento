package node

import (
	"net/url"
	"strings"
)

// weburl backs the WHATWG URL global. Parsing a URL correctly (percent
// encoding, IDNA hosts, default ports, base resolution) is a lot of surface to
// reimplement in JavaScript, and Go's net/url already does it, so the URL class
// keeps its state as fields the JavaScript side reads and only calls back here
// to (re)parse. There is no I/O, so this registers alongside the pure host
// functions rather than through the loop-aware net install.

// urlHostFuncs returns the URL parsing host function. It is pure (no loop, no
// I/O), so HostFuncs bundles it with fs and os.
func urlHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_url_parse": urlParse,
	}
}

// urlParse resolves input against an optional base and returns the parsed
// components as a JSON object. On failure it returns {"ok":false} so the URL
// constructor can throw a TypeError, matching the WHATWG contract that an
// invalid URL is a hard error rather than a null result.
func urlParse(args []any) (any, error) {
	input := strings.TrimSpace(str(args, 0))
	base := str(args, 1)

	var u *url.URL
	var err error
	if base != "" {
		b, berr := url.Parse(base)
		if berr != nil || !b.IsAbs() {
			return urlFail(), nil
		}
		u, err = b.Parse(input)
	} else {
		u, err = url.Parse(input)
	}
	if err != nil || !u.IsAbs() {
		return urlFail(), nil
	}

	return jsonString(urlComponents(u)), nil
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

func urlFail() string { return jsonString(urlResult{OK: false}) }

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
