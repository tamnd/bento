package value

import "testing"

func TestNewURLComponents(t *testing.T) {
	u := NewURL(FromGoString("https://user:pw@example.com:8443/a/b?x=1&y=2#frag"))
	for _, c := range []struct {
		name string
		got  BStr
		want string
	}{
		{"href", u.Href(), "https://user:pw@example.com:8443/a/b?x=1&y=2#frag"},
		{"protocol", u.Protocol(), "https:"},
		{"username", u.Username(), "user"},
		{"password", u.Password(), "pw"},
		{"host", u.Host(), "example.com:8443"},
		{"hostname", u.Hostname(), "example.com"},
		{"port", u.Port(), "8443"},
		{"pathname", u.Pathname(), "/a/b"},
		{"search", u.Search(), "?x=1&y=2"},
		{"hash", u.Hash(), "#frag"},
		{"origin", u.Origin(), "https://example.com:8443"},
	} {
		if got := c.got.ToGoString(); got != c.want {
			t.Errorf("url.%s = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNewURLResolvesAgainstBase(t *testing.T) {
	u := NewURL(FromGoString("../c?q=1"), FromGoString("https://example.com/a/b/d"))
	if got := u.Href().ToGoString(); got != "https://example.com/a/c?q=1" {
		t.Errorf("relative href = %q, want https://example.com/a/c?q=1", got)
	}
}

func TestNewURLDefaultsPathAndOrigin(t *testing.T) {
	u := NewURL(FromGoString("http://example.com"))
	if got := u.Pathname().ToGoString(); got != "/" {
		t.Errorf("pathname of a URL with no path = %q, want /", got)
	}
	if got := u.Port().ToGoString(); got != "" {
		t.Errorf("port of a default-port URL = %q, want empty", got)
	}
	// A scheme with no tuple origin serializes its origin as the string "null".
	if got := NewURL(FromGoString("mailto:a@b.example")).Origin().ToGoString(); got != "null" {
		t.Errorf("origin of an opaque scheme = %q, want null", got)
	}
}

func TestNewURLThrowsOnInvalidInput(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("new URL of a relative input with no base did not throw")
		}
	}()
	NewURL(FromGoString("not a url"))
}

func TestURLCanParse(t *testing.T) {
	if !URLCanParse(FromGoString("https://example.com")) {
		t.Error("canParse of an absolute URL = false, want true")
	}
	if URLCanParse(FromGoString("/relative")) {
		t.Error("canParse of a relative input with no base = true, want false")
	}
	if !URLCanParse(FromGoString("/relative"), FromGoString("https://example.com")) {
		t.Error("canParse of a relative input with a base = false, want true")
	}
}

func TestURLToStringAndToJSON(t *testing.T) {
	u := NewURL(FromGoString("https://example.com/p?a=1"))
	if got := u.ToString().ToGoString(); got != "https://example.com/p?a=1" {
		t.Errorf("toString = %q", got)
	}
	if got := u.ToJSON().ToGoString(); got != "https://example.com/p?a=1" {
		t.Errorf("toJSON = %q", got)
	}
}

func TestURLSearchParamsParsesTheQuery(t *testing.T) {
	u := NewURL(FromGoString("https://example.com/?a=1&b=2&a=3&flag"))
	p := u.SearchParams()
	if got := p.Size(); got != 4 {
		t.Errorf("size = %v, want 4", got)
	}
	if got := p.Get(FromGoString("a")); got.IsNull() || got.AsString().ToGoString() != "1" {
		t.Errorf("get(a) = %v, want 1", got)
	}
	all := p.GetAll(FromGoString("a"))
	if all.Len() != 2 {
		t.Fatalf("getAll(a) length = %v, want 2", all.Len())
	}
	if got := all.Elems()[1].ToGoString(); got != "3" {
		t.Errorf("getAll(a)[1] = %q, want 3", got)
	}
	// A segment with no "=" is a name with an empty value.
	if got := p.Get(FromGoString("flag")); got.IsNull() || got.AsString().ToGoString() != "" {
		t.Errorf("get(flag) = %v, want the empty string", got)
	}
	if !p.Get(FromGoString("missing")).IsNull() {
		t.Error("get of an absent name did not report null")
	}
	if p.Has(FromGoString("missing")) {
		t.Error("has of an absent name = true")
	}
}

func TestURLSearchParamsIsALiveViewOfItsURL(t *testing.T) {
	u := NewURL(FromGoString("https://example.com/p?a=1"))
	p := u.SearchParams()
	p.Append(FromGoString("b"), FromGoString("2"))
	if got := u.Search().ToGoString(); got != "?a=1&b=2" {
		t.Errorf("search after append = %q, want ?a=1&b=2", got)
	}
	if got := u.Href().ToGoString(); got != "https://example.com/p?a=1&b=2" {
		t.Errorf("href after append = %q", got)
	}
	p.Delete(FromGoString("a"))
	p.Delete(FromGoString("b"))
	if got := u.Search().ToGoString(); got != "" {
		t.Errorf("search after deleting every parameter = %q, want empty", got)
	}
	if got := u.Href().ToGoString(); got != "https://example.com/p" {
		t.Errorf("href after deleting every parameter = %q", got)
	}
}

func TestURLSearchParamsSetReplacesInPlace(t *testing.T) {
	p := NewURLSearchParams(FromGoString("a=1&b=2&a=3"))
	p.Set(FromGoString("a"), FromGoString("9"))
	if got := p.ToString().ToGoString(); got != "a=9&b=2" {
		t.Errorf("after set(a,9) = %q, want a=9&b=2", got)
	}
	p.Set(FromGoString("c"), FromGoString("4"))
	if got := p.ToString().ToGoString(); got != "a=9&b=2&c=4" {
		t.Errorf("after set of a new name = %q", got)
	}
}

func TestURLSearchParamsSortIsStable(t *testing.T) {
	p := NewURLSearchParams(FromGoString("b=1&a=2&b=3&a=4"))
	p.Sort()
	if got := p.ToString().ToGoString(); got != "a=2&a=4&b=1&b=3" {
		t.Errorf("after sort = %q, want a=2&a=4&b=1&b=3", got)
	}
}

func TestURLSearchParamsForEach(t *testing.T) {
	p := NewURLSearchParams(FromGoString("a=1&b=2"))
	var pairs, values string
	p.ForEach(func(value, name BStr) {
		pairs += name.ToGoString() + "=" + value.ToGoString() + ";"
	})
	if pairs != "a=1;b=2;" {
		t.Errorf("forEach walked %q, want a=1;b=2;", pairs)
	}
	p.ForEachValue(func(value BStr) { values += value.ToGoString() })
	if values != "12" {
		t.Errorf("forEach over values walked %q, want 12", values)
	}
}

func TestURLSearchParamsEncoding(t *testing.T) {
	p := NewURLSearchParams()
	p.Append(FromGoString("a b"), FromGoString("c&d=e"))
	p.Append(FromGoString("π"), FromGoString("*!'()"))
	// Space is "+", and the characters encodeURIComponent spares but the query
	// serializer does not are percent encoded.
	want := "a+b=c%26d%3De&%CF%80=%2A%21%27%28%29"
	if got := p.ToString().ToGoString(); got != want {
		t.Errorf("serialization = %q, want %q", got, want)
	}
	// And it round-trips.
	back := NewURLSearchParams(FromGoString(want))
	if got := back.Get(FromGoString("a b")); got.IsNull() || got.AsString().ToGoString() != "c&d=e" {
		t.Errorf("round-tripped value = %v, want c&d=e", got)
	}
	if got := back.Get(FromGoString("π")); got.IsNull() || got.AsString().ToGoString() != "*!'()" {
		t.Errorf("round-tripped unicode name = %v", got)
	}
}

func TestURLSearchParamsDecodeIsLenient(t *testing.T) {
	// A "%" that does not begin a valid escape stays literal, and bytes that do not
	// form valid UTF-8 become the replacement character, rather than throwing.
	p := NewURLSearchParams(FromGoString("a=100%&b=%FF"))
	if got := p.Get(FromGoString("a")); got.IsNull() || got.AsString().ToGoString() != "100%" {
		t.Errorf("get(a) = %v, want 100%%", got)
	}
	if got := p.Get(FromGoString("b")); got.IsNull() || got.AsString().ToGoString() != "�" {
		t.Errorf("get(b) = %v, want the replacement character", got)
	}
}

func TestNewURLSearchParamsFromCopyIsIndependent(t *testing.T) {
	u := NewURL(FromGoString("https://example.com/?a=1"))
	copyOf := NewURLSearchParamsFrom(u.SearchParams())
	copyOf.Append(FromGoString("b"), FromGoString("2"))
	if got := copyOf.ToString().ToGoString(); got != "a=1&b=2" {
		t.Errorf("the copy = %q, want a=1&b=2", got)
	}
	if got := u.Search().ToGoString(); got != "?a=1" {
		t.Errorf("mutating the copy moved the source URL's search to %q", got)
	}
}

func TestURLSearchParamsStripsALeadingQuestionMark(t *testing.T) {
	p := NewURLSearchParams(FromGoString("?a=1"))
	if got := p.ToString().ToGoString(); got != "a=1" {
		t.Errorf("a query given with its leading ? = %q, want a=1", got)
	}
}
