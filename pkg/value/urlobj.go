package value

// This file gives the AOT runtime the WHATWG URL and URLSearchParams pair, the two
// classes Node and the web platform expose as globals and as the node:url module.
// Until now a compiled program that wrote `new URL(...)` handed back to the
// interpreter, which is a shame: URL is one of the first things a real .js entry
// reaches for, and the parse it needs already exists in Go.
//
// The parsing is not repeated here. pkg/nodehost owns it, over net/url, and the
// interpreter reaches the same function through a JSON bridge. This file is the
// object surface: the eleven components a program reads, the query serialization
// URLSearchParams defines, and the live view that keeps url.search and
// url.searchParams from ever disagreeing.
//
// Both lower to Go types rather than to boxed property bags. searchParams returns a
// URLSearchParams, get returns a string-or-null, and getAll returns an array, all
// typed-world values, so a URL held in a value.Value could not hand them back. The
// compiler knows the receiver at every call site, so the monomorphic form is also
// the direct one, the same reasoning TextEncoder and Date take.

import (
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/nodehost"
)

// URL is a parsed absolute URL, the lowering of new URL(input, base). It holds the
// components the specification exposes as getters, plus the searchParams view built
// with it. The view holds a back-pointer here, so a mutation through it re-serializes
// search and href: a program that appends a parameter and then reads url.href sees
// the parameter, which is the whole point of the live view.
type URL struct {
	href     string
	protocol string
	username string
	password string
	host     string
	hostname string
	port     string
	pathname string
	search   string
	hash     string
	origin   string
	params   *URLSearchParams
}

// NewURL parses input, optionally against a base, the lowering of new URL(input) and
// new URL(input, base). An input that is not a valid absolute URL throws a TypeError,
// which is what the specification requires: an invalid URL is a hard error, not a null
// result. Only the first base is read; the variadic is how the optional second
// argument reaches a Go signature.
func NewURL(input BStr, base ...BStr) *URL {
	baseStr := ""
	if len(base) > 0 {
		baseStr = base[0].ToGoString()
	}
	parts, ok := nodehost.URLParse(input.ToGoString(), baseStr)
	if !ok {
		Throw(NewTypeError(Concat(FromGoString("Invalid URL: "), input)))
	}
	u := &URL{
		href:     parts.Href,
		protocol: parts.Protocol,
		username: parts.Username,
		password: parts.Password,
		host:     parts.Host,
		hostname: parts.Hostname,
		port:     parts.Port,
		pathname: parts.Pathname,
		search:   parts.Search,
		hash:     parts.Hash,
		origin:   parts.Origin,
	}
	u.params = &URLSearchParams{url: u}
	u.params.parseQuery(u.search)
	return u
}

// URLCanParse reports whether new URL would succeed, the lowering of the static
// URL.canParse(input, base). It is the same parse without the throw, which is why the
// specification added it: asking the question should not cost an exception.
func URLCanParse(input BStr, base ...BStr) bool {
	baseStr := ""
	if len(base) > 0 {
		baseStr = base[0].ToGoString()
	}
	_, ok := nodehost.URLParse(input.ToGoString(), baseStr)
	return ok
}

// The eleven component getters. Each is an accessor in the source and a method here,
// the same shape map.size takes.

// Href is the serialized URL, the lowering of url.href.
func (u *URL) Href() BStr { return FromGoString(u.href) }

// Protocol is the scheme with its trailing colon, "https:", the lowering of
// url.protocol.
func (u *URL) Protocol() BStr { return FromGoString(u.protocol) }

// Username is the userinfo name, empty when there is none, the lowering of
// url.username.
func (u *URL) Username() BStr { return FromGoString(u.username) }

// Password is the userinfo password, empty when there is none, the lowering of
// url.password.
func (u *URL) Password() BStr { return FromGoString(u.password) }

// Host is the hostname with the port when one is given, the lowering of url.host.
func (u *URL) Host() BStr { return FromGoString(u.host) }

// Hostname is the host without the port, the lowering of url.hostname.
func (u *URL) Hostname() BStr { return FromGoString(u.hostname) }

// Port is the port, empty when the URL uses its scheme's default, the lowering of
// url.port.
func (u *URL) Port() BStr { return FromGoString(u.port) }

// Pathname is the path, "/" for an absolute URL with no path, the lowering of
// url.pathname.
func (u *URL) Pathname() BStr { return FromGoString(u.pathname) }

// Search is the query with its leading "?", empty when there is no query, the
// lowering of url.search. It reflects any mutation made through searchParams.
func (u *URL) Search() BStr { return FromGoString(u.search) }

// Hash is the fragment with its leading "#", empty when there is none, the lowering
// of url.hash.
func (u *URL) Hash() BStr { return FromGoString(u.hash) }

// Origin is the scheme, host and port triple for a special scheme and the string
// "null" for any other, the lowering of url.origin. It is "null" and not the null
// value because the specification serializes an opaque origin that way.
func (u *URL) Origin() BStr { return FromGoString(u.origin) }

// SearchParams is the live view over the query, the lowering of url.searchParams. It
// is the same object every read, as the specification requires, so a caller can hold
// it and keep mutating through it.
func (u *URL) SearchParams() *URLSearchParams { return u.params }

// ToString is the serialized URL, the lowering of url.toString(). ToJSON is the same
// string, which is what the specification defines for url.toJSON(), so JSON.stringify
// of a URL gives its href.
func (u *URL) ToString() BStr { return FromGoString(u.href) }

// ToJSON is the lowering of url.toJSON(), the serialized URL.
func (u *URL) ToJSON() BStr { return FromGoString(u.href) }

// setSearch takes a serialized query back from the view and rebuilds search and href
// around it. It runs on every mutation through searchParams, which is what keeps the
// two readings of the query in agreement.
func (u *URL) setSearch(serialized string) {
	if serialized == "" {
		u.search = ""
	} else {
		u.search = "?" + serialized
	}
	u.href = u.rebuildHref()
}

// rebuildHref reassembles the serialization from the components. Only the query moves
// today, so the other pieces are written back exactly as the parse produced them.
func (u *URL) rebuildHref() string {
	auth := ""
	if u.username != "" {
		auth = u.username
		if u.password != "" {
			auth += ":" + u.password
		}
		auth += "@"
	}
	return u.protocol + "//" + auth + u.host + u.pathname + u.search + u.hash
}

// urlParam is one name/value pair of a query. The list keeps insertion order, which
// every URLSearchParams operation but sort preserves and which its serialization
// depends on.
type urlParam struct {
	name  BStr
	value BStr
}

// URLSearchParams is an ordered list of query parameters, the lowering of
// new URLSearchParams(init). It is a multimap, not a map: a query can repeat a name,
// and getAll exists precisely to read the repeats, so the storage is a list and not a
// Go map.
//
// url is set when this view belongs to a URL and nil for a standalone one. Every
// mutating method calls sync, so an owned view writes its serialization back through
// the owner.
type URLSearchParams struct {
	list []urlParam
	url  *URL
}

// NewURLSearchParams builds a view from an optional query string, the lowering of
// new URLSearchParams() and new URLSearchParams(query). A leading "?" is stripped, as
// the specification requires, so both "a=1" and "?a=1" parse the same.
func NewURLSearchParams(query ...BStr) *URLSearchParams {
	p := &URLSearchParams{}
	if len(query) > 0 {
		p.parseQuery(query[0].ToGoString())
	}
	return p
}

// NewURLSearchParamsFrom copies another view's pairs, the lowering of
// new URLSearchParams(other). The copy is independent and unowned: mutating it never
// touches the source or the source's URL.
func NewURLSearchParamsFrom(other *URLSearchParams) *URLSearchParams {
	p := &URLSearchParams{list: make([]urlParam, len(other.list))}
	copy(p.list, other.list)
	return p
}

// parseQuery fills the list from a serialized query. An empty segment is skipped, and
// a segment with no "=" is a name with an empty value, both as the specification says.
func (p *URLSearchParams) parseQuery(query string) {
	query = strings.TrimPrefix(query, "?")
	if query == "" {
		return
	}
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		name, value, found := strings.Cut(part, "=")
		if !found {
			value = ""
		}
		p.list = append(p.list, urlParam{
			name:  FromGoString(decodeFormComponent(name)),
			value: FromGoString(decodeFormComponent(value)),
		})
	}
}

// sync pushes this view's serialization onto the owning URL, so url.search and
// url.searchParams never disagree. A standalone view has no owner and does nothing.
func (p *URLSearchParams) sync() {
	if p.url != nil {
		p.url.setSearch(p.serialize())
	}
}

// Append adds a pair without disturbing an existing one of the same name, the
// lowering of params.append(name, value).
func (p *URLSearchParams) Append(name, value BStr) {
	p.list = append(p.list, urlParam{name: name, value: value})
	p.sync()
}

// Delete removes every pair with the given name, the lowering of
// params.delete(name).
func (p *URLSearchParams) Delete(name BStr) {
	kept := p.list[:0]
	for _, pair := range p.list {
		if !pair.name.Equal(name) {
			kept = append(kept, pair)
		}
	}
	p.list = kept
	p.sync()
}

// Get is the first value for a name, the lowering of params.get(name). It returns a
// boxed Value rather than a BStr because the specification's answer for an absent name
// is null, not undefined or the empty string, and null is a value of its own: there is
// no BStr that means "no such parameter". So the result carries the string or Null, and
// the caller reads it on the dynamic path, the same shape re.exec's array-or-null takes.
func (p *URLSearchParams) Get(name BStr) Value {
	for _, pair := range p.list {
		if pair.name.Equal(name) {
			return StringValue(pair.value)
		}
	}
	return Null
}

// GetAll is every value for a name in insertion order, the lowering of
// params.getAll(name). An absent name gives an empty array, not null: this is the
// method that reads the repeats a query is allowed to carry.
func (p *URLSearchParams) GetAll(name BStr) *Array[BStr] {
	var out []BStr
	for _, pair := range p.list {
		if pair.name.Equal(name) {
			out = append(out, pair.value)
		}
	}
	return ArrayFrom(out)
}

// Has reports whether any pair carries the name, the lowering of params.has(name).
func (p *URLSearchParams) Has(name BStr) bool {
	for _, pair := range p.list {
		if pair.name.Equal(name) {
			return true
		}
	}
	return false
}

// Set replaces the first pair with the name and drops the rest, appending when the
// name is absent, the lowering of params.set(name, value). Keeping the first pair's
// position rather than moving it to the end is what the specification requires, and it
// is what makes set idempotent on the serialization.
func (p *URLSearchParams) Set(name, value BStr) {
	found := false
	kept := p.list[:0]
	for _, pair := range p.list {
		switch {
		case !pair.name.Equal(name):
			kept = append(kept, pair)
		case !found:
			kept = append(kept, urlParam{name: name, value: value})
			found = true
		}
	}
	p.list = kept
	if !found {
		p.list = append(p.list, urlParam{name: name, value: value})
	}
	p.sync()
}

// Sort orders the pairs by name, the lowering of params.sort(). The sort is stable, so
// pairs that repeat a name keep their relative order, which the specification requires.
// Names compare by UTF-16 code unit, the ordering JavaScript's own relational operator
// gives, not by Go's byte ordering, which differs for the surrogate range.
func (p *URLSearchParams) Sort() {
	sort.SliceStable(p.list, func(i, j int) bool {
		return p.list[i].name.Compare(p.list[j].name) < 0
	})
	p.sync()
}

// Size is the pair count, the lowering of the params.size accessor. It counts pairs
// and not distinct names, so a query that repeats a name counts it each time.
func (p *URLSearchParams) Size() float64 { return float64(len(p.list)) }

// ForEach walks the pairs in order passing the value and then the name, the argument
// order the specification's callback takes, the lowering of a two-parameter
// params.forEach(cb).
func (p *URLSearchParams) ForEach(fn func(value, name BStr)) {
	for _, pair := range p.list {
		fn(pair.value, pair.name)
	}
}

// ForEachValue walks the pairs passing only the value, the lowering of a
// one-parameter params.forEach(cb). It is the common shape, so it gets a callback with
// no unused parameter, the same split map.forEach takes.
func (p *URLSearchParams) ForEachValue(fn func(value BStr)) {
	for _, pair := range p.list {
		fn(pair.value)
	}
}

// ToString is the form-urlencoded serialization, the lowering of params.toString().
func (p *URLSearchParams) ToString() BStr { return FromGoString(p.serialize()) }

// serialize renders the pairs as application/x-www-form-urlencoded.
func (p *URLSearchParams) serialize() string {
	var b strings.Builder
	for i, pair := range p.list {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(encodeFormComponent(pair.name.ToGoString()))
		b.WriteByte('=')
		b.WriteString(encodeFormComponent(pair.value.ToGoString()))
	}
	return b.String()
}

const upperHex = "0123456789ABCDEF"

// encodeFormComponent percent-encodes one name or value for a query string. The set
// left literal is the urlencoded serializer's, which is not the unreserved set and not
// encodeURIComponent's: a space is "+", "*" stays literal, and "~" does not. Those last
// two are each the opposite of what encodeURIComponent does, so the set is spelled out
// here rather than described as a delta from it.
func encodeFormComponent(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '*', c == '-', c == '.', c == '_':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String()
}

// decodeFormComponent reverses encodeFormComponent: "+" is a space and a percent
// escape is its byte. A "%" that is not followed by two hex digits stays literal, and
// bytes that do not form valid UTF-8 become the replacement character, rather than
// either throwing or handing back a Go string that is not valid UTF-8. That is what
// Node does with a malformed query, and it matters here because a query string arrives
// from the network and cannot be assumed well formed.
func decodeFormComponent(s string) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '+':
			b.WriteByte(' ')
		case s[i] == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]):
			b.WriteByte(hexValue(s[i+1])<<4 | hexValue(s[i+2]))
			i += 2
		default:
			b.WriteByte(s[i])
		}
	}
	return strings.ToValidUTF8(b.String(), "�")
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexValue(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}
