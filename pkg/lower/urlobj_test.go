package lower

import (
	"strings"
	"testing"
)

// TestNewURLLowers pins that new URL(input) reaches value.NewURL rather than handing
// back at the constructor gate, and that a base lowers through as the second argument
// the runtime's variadic takes.
func TestNewURLLowers(t *testing.T) {
	src := `const u = new URL("https://example.com/p?a=1"); const h: string = u.href;`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.NewURL(value.FromGoString("https://example.com/p?a=1"))`) {
		t.Fatalf("new URL did not lower to value.NewURL:\n%s", out)
	}
	if !strings.Contains(out, ".Href()") {
		t.Fatalf("url.href did not lower to the Href method:\n%s", out)
	}
}

// TestNewURLWithBaseLowers pins the two-argument construction, which resolves a
// relative input against a base.
func TestNewURLWithBaseLowers(t *testing.T) {
	src := `const u = new URL("/p", "https://example.com"); const h: string = u.href;`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.NewURL(value.FromGoString("/p"), value.FromGoString("https://example.com"))`) {
		t.Fatalf("new URL with a base did not lower with both arguments:\n%s", out)
	}
}

// TestURLGettersLower pins that every component the specification exposes reaches its
// runtime method, so a program can read the whole parse and not just the parts the
// first fixture happened to touch.
func TestURLGettersLower(t *testing.T) {
	src := `const u = new URL("https://a:b@example.com:8443/p?q=1#f");
const parts: string[] = [u.protocol, u.username, u.password, u.host, u.hostname, u.port, u.pathname, u.search, u.hash, u.origin];`
	out := renderProgram(t, src)
	for _, want := range []string{
		".Protocol()", ".Username()", ".Password()", ".Host()", ".Hostname()",
		".Port()", ".Pathname()", ".Search()", ".Hash()", ".Origin()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in the emitted program:\n%s", want, out)
		}
	}
}

// TestURLCanParseLowers pins the static, which asks whether construction would succeed
// without paying for the exception.
func TestURLCanParseLowers(t *testing.T) {
	src := `const ok: boolean = URL.canParse("https://example.com");`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.URLCanParse(") {
		t.Fatalf("URL.canParse did not lower to value.URLCanParse:\n%s", out)
	}
}

// TestURLToStringAndToJSONLower pins the two methods a URL carries; every other member
// is an accessor.
func TestURLToStringAndToJSONLower(t *testing.T) {
	src := `const u = new URL("https://example.com/"); const a: string = u.toString(); const b: string = u.toJSON();`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ToString()") || !strings.Contains(out, ".ToJSON()") {
		t.Fatalf("url.toString/toJSON did not lower to their methods:\n%s", out)
	}
}

// TestURLSearchParamsMethodsLower pins the mutating and reading surface of the query
// view, including size, which is an accessor in the source and a method on the runtime.
func TestURLSearchParamsMethodsLower(t *testing.T) {
	src := `const p = new URLSearchParams("a=1");
p.append("b", "2");
p.set("a", "9");
p.delete("b");
p.sort();
const all: string[] = p.getAll("a");
const has: boolean = p.has("a");
const n: number = p.size;
const s: string = p.toString();`
	out := renderProgram(t, src)
	for _, want := range []string{
		`value.NewURLSearchParams(value.FromGoString("a=1"))`,
		".Append(", ".Set(", ".Delete(", ".Sort()", ".GetAll(", ".Has(", ".Size()", ".ToString()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in the emitted program:\n%s", want, out)
		}
	}
}

// TestURLSearchParamsGetIsBoxed pins that params.get(name) stays on the dynamic path.
// The checker types it string | null, which bento renders as a tagged sum the runtime
// cannot name, so the call returns the boxed value.Value and its binding is a
// value.Value slot, the same treatment re.exec's array-or-null takes.
func TestURLSearchParamsGetIsBoxed(t *testing.T) {
	src := `const p = new URLSearchParams("a=1"); const v = p.get("a"); console.log(v === null);`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var v value.Value = ") {
		t.Fatalf("params.get did not bind through a boxed value.Value slot:\n%s", out)
	}
	if !strings.Contains(out, ".Get(") {
		t.Fatalf("params.get did not lower to the Get method:\n%s", out)
	}
}

// TestURLSearchParamsIsALiveView pins that url.searchParams reaches the view rather
// than interning searchParams as a struct field of a shape a URL has not.
func TestURLSearchParamsIsALiveView(t *testing.T) {
	src := `const u = new URL("https://example.com/?a=1"); u.searchParams.append("b", "2"); const s: string = u.search;`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".SearchParams().Append(") {
		t.Fatalf("url.searchParams.append did not lower through the view:\n%s", out)
	}
}

// TestURLSearchParamsForEachLowers pins both callback shapes: a one-parameter arrow
// reads the value, and a two-parameter arrow reads the value then the name, the order
// the specification passes them.
func TestURLSearchParamsForEachLowers(t *testing.T) {
	one := renderProgram(t, `const p = new URLSearchParams("a=1"); p.forEach((v) => { console.log(v); });`)
	if !strings.Contains(one, ".ForEachValue(func(") {
		t.Fatalf("a one-parameter forEach did not lower to ForEachValue:\n%s", one)
	}
	two := renderProgram(t, `const p = new URLSearchParams("a=1"); p.forEach((v, k) => { console.log(k + "=" + v); });`)
	if !strings.Contains(two, ".ForEach(func(") {
		t.Fatalf("a two-parameter forEach did not lower to ForEach:\n%s", two)
	}
}

// TestNewURLSearchParamsFromCopyLowers pins the copy constructor, which takes another
// view's pairs and is not owned by any URL.
func TestNewURLSearchParamsFromCopyLowers(t *testing.T) {
	src := `const a = new URLSearchParams("x=1"); const b = new URLSearchParams(a); const s: string = b.toString();`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewURLSearchParamsFrom(") {
		t.Fatalf("new URLSearchParams(other) did not lower to the copy constructor:\n%s", out)
	}
}

// TestURLSearchParamsIteratorHandsBack pins that keys(), values() and entries() hand
// back naming themselves rather than lowering to something that drops the iterator.
func TestURLSearchParamsIteratorHandsBack(t *testing.T) {
	for _, method := range []string{"keys", "values", "entries"} {
		reason := renderProgramHandBack(t, `const p = new URLSearchParams("a=1"); p.`+method+`();`)
		if !strings.Contains(reason, "URLSearchParams ."+method+"()") {
			t.Errorf("a params.%s() iterator handed back for the wrong reason: %q", method, reason)
		}
	}
}

// TestURLSearchParamsForOfHandsBack pins that ranging a query view directly, or through
// one of its iterator methods, hands back rather than being claimed by the Map for...of
// interception. The query view carries a Map's get/set/has/size shape, so before this
// slice ruled it out of the Map fingerprint the loop emitted a p.Keys() call the runtime
// type does not have, which is generated Go that does not compile.
func TestURLSearchParamsForOfHandsBack(t *testing.T) {
	for _, iterable := range []string{"p", "p.keys()", "p.entries()"} {
		src := `const p = new URLSearchParams("a=1"); for (const k of ` + iterable + `) { console.log(String(k)); }`
		if reason := renderProgramHandBack(t, src); reason == "" {
			t.Errorf("for...of over %s lowered rather than handing back", iterable)
		}
	}
}

// TestURLSearchParamsDeleteWithValueHandsBack pins the two-argument delete, which
// filters on the value as well. The runtime takes the name alone, so lowering it would
// delete every pair with the name and not just the matching one.
func TestURLSearchParamsDeleteWithValueHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, `const p = new URLSearchParams("a=1"); p.delete("a", "1");`)
	if !strings.Contains(reason, "filters on the value") {
		t.Fatalf("params.delete(name, value) handed back for the wrong reason: %q", reason)
	}
}

// TestNewURLNonStringHandsBack pins that a URL built from a value the specification
// would coerce through String() hands back rather than reach a constructor with no slot
// for it.
func TestNewURLNonStringHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, `const n: any = 5; const u = new URL(n); console.log(u.href);`)
	if !strings.Contains(reason, "needs coercion") {
		t.Fatalf("new URL over a coerced value handed back for the wrong reason: %q", reason)
	}
}

// TestURLRunsThroughTheAOTPath builds and runs a program that parses a URL, reads its
// components, and mutates the query through the live view, so the whole slice is proven
// end to end and not just at the emitted-Go level.
func TestURLRunsThroughTheAOTPath(t *testing.T) {
	skipIfShort(t)
	src := `const u = new URL("https://user@example.com:8443/a/b?x=1#f");
console.log(u.protocol);
console.log(u.hostname);
console.log(u.port);
console.log(u.pathname);
console.log(u.search);
console.log(u.hash);
console.log(u.origin);
u.searchParams.append("y", "two words");
console.log(u.search);
console.log(u.href);
console.log(String(u.searchParams.size));`
	want := strings.Join([]string{
		"https:",
		"example.com",
		"8443",
		"/a/b",
		"?x=1",
		"#f",
		"https://example.com:8443",
		"?x=1&y=two+words",
		"https://user@example.com:8443/a/b?x=1&y=two+words#f",
		"2",
		"",
	}, "\n")
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("URL run mismatch:\n got %q\nwant %q", got, want)
	}
}
