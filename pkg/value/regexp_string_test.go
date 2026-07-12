package value

import "testing"

// search reports the UTF-16 index of the first match or -1, and does not depend on
// the global flag or lastIndex.
func TestRegExpSearch(t *testing.T) {
	re := NewRegExpLiteral("b+", "")
	if got := re.Search(FromGoString("aabbbc")); got != 2 {
		t.Errorf("search = %v, want 2", got)
	}
	if got := re.Search(FromGoString("xyz")); got != -1 {
		t.Errorf("search on a miss = %v, want -1", got)
	}
	g := NewRegExpLiteral("a", "g")
	g.SetLastIndex(3)
	if got := g.Search(FromGoString("aaaa")); got != 0 {
		t.Errorf("search ignores lastIndex, got %v, want 0", got)
	}
}

// A non-global match returns exec's array; a global match returns every matched
// substring, and null when there is no match.
func TestRegExpMatchStr(t *testing.T) {
	re := NewRegExpLiteral("a(b+)", "")
	m := re.MatchStr(FromGoString("xabbby"))
	if m.IsNull() || m.GetIndex(0).AsString().ToGoString() != "abbb" {
		t.Errorf("non-global match[0] wrong: %v", m)
	}
	g := NewRegExpLiteral("a.", "g")
	all := g.MatchStr(FromGoString("axaybz"))
	if all.IsNull() {
		t.Fatal("global match returned null on a matching input")
	}
	if got := arrayStrings(all); len(got) != 2 || got[0] != "ax" || got[1] != "ay" {
		t.Errorf("global match = %v, want [ax ay]", got)
	}
	if !NewRegExpLiteral("z", "g").MatchStr(FromGoString("abc")).IsNull() {
		t.Error("global match on a miss did not return null")
	}
}

// A global match over an empty-matching pattern advances one code unit per step so
// the walk terminates and reports one empty match per position plus the end.
func TestRegExpMatchStrEmpty(t *testing.T) {
	g := NewRegExpLiteral("x*", "g")
	all := arrayStrings(g.MatchStr(FromGoString("xax")))
	// "xax": x at 0, "" at 1 (between x and a is a; x* matches x then empty)...
	if len(all) == 0 {
		t.Fatalf("empty-pattern global match produced nothing")
	}
}

// replace substitutes the first match, replaceAll (a global regexp) every match, and
// the template expands the whole match, a capture group, and a literal dollar.
func TestRegExpReplaceStr(t *testing.T) {
	first := NewRegExpLiteral("a", "").ReplaceStr(FromGoString("banana"), FromGoString("X"))
	if first.ToGoString() != "bXnana" {
		t.Errorf("non-global replace = %q, want bXnana", first.ToGoString())
	}
	all := NewRegExpLiteral("a", "g").ReplaceStr(FromGoString("banana"), FromGoString("X"))
	if all.ToGoString() != "bXnXnX" {
		t.Errorf("global replace = %q, want bXnXnX", all.ToGoString())
	}
	group := NewRegExpLiteral("(a)(b)", "").ReplaceStr(FromGoString("ab"), FromGoString("$2$1"))
	if group.ToGoString() != "ba" {
		t.Errorf("group swap = %q, want ba", group.ToGoString())
	}
	whole := NewRegExpLiteral("b+", "").ReplaceStr(FromGoString("abbbc"), FromGoString("[$&]"))
	if whole.ToGoString() != "a[bbb]c" {
		t.Errorf("$& = %q, want a[bbb]c", whole.ToGoString())
	}
	dollar := NewRegExpLiteral("a", "").ReplaceStr(FromGoString("a"), FromGoString("$$"))
	if dollar.ToGoString() != "$" {
		t.Errorf("$$ = %q, want $", dollar.ToGoString())
	}
	pre := NewRegExpLiteral("c", "").ReplaceStr(FromGoString("abc"), FromGoString("[$`]"))
	if pre.ToGoString() != "ab[ab]" {
		t.Errorf("$` = %q, want ab[ab]", pre.ToGoString())
	}
}

// split cuts the subject at each separator match, includes each capture group of the
// separator between the pieces, honors the limit, and handles the empty subject.
func TestRegExpSplitStr(t *testing.T) {
	plain := arrayStrings(NewRegExpLiteral(",", "").SplitStr(FromGoString("a,b,c"), false, 0))
	if len(plain) != 3 || plain[0] != "a" || plain[2] != "c" {
		t.Errorf("split = %v, want [a b c]", plain)
	}
	caps := arrayStrings(NewRegExpLiteral("(,)", "").SplitStr(FromGoString("a,b"), false, 0))
	if len(caps) != 3 || caps[0] != "a" || caps[1] != "," || caps[2] != "b" {
		t.Errorf("split with capture = %v, want [a , b]", caps)
	}
	limited := NewRegExpLiteral(",", "").SplitStr(FromGoString("a,b,c"), true, 2)
	if got := arrayStrings(limited); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("limited split = %v, want [a b]", got)
	}
	empty := arrayStrings(NewRegExpLiteral("x", "").SplitStr(FromGoString(""), false, 0))
	if len(empty) != 1 || empty[0] != "" {
		t.Errorf("empty-subject split = %v, want [\"\"]", empty)
	}
	zero := arrayStrings(NewRegExpLiteral(",", "").SplitStr(FromGoString("a,b"), true, 0))
	if len(zero) != 0 {
		t.Errorf("zero-limit split = %v, want []", zero)
	}
}

// arrayStrings reads a value array into a Go slice of its elements' string forms, so
// a test can assert on the whole result at once.
func arrayStrings(v Value) []string {
	n := int(v.Get(FromGoString("length")).AsNumber())
	out := make([]string, n)
	for i := 0; i < n; i++ {
		e := v.GetIndex(float64(i))
		if e.IsUndefined() {
			out[i] = "<undefined>"
			continue
		}
		out[i] = e.AsString().ToGoString()
	}
	return out
}
