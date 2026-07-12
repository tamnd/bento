package value

import (
	"regexp"
	"testing"
)

// The flag parser accepts each defined flag once, imposes no order, and rejects an
// unknown flag, a repeated flag, and the mutually exclusive u and v pair, the three
// SyntaxError cases the specification names.
func TestParseRegExpFlags(t *testing.T) {
	fl, ok := parseRegExpFlags("gimsy")
	if !ok || !fl.global || !fl.ignoreCase || !fl.multiline || !fl.dotAll || !fl.sticky {
		t.Fatalf("parseRegExpFlags(gimsy) = %+v, %v", fl, ok)
	}
	if _, ok := parseRegExpFlags("gg"); ok {
		t.Fatal("a repeated flag parsed as valid")
	}
	if _, ok := parseRegExpFlags("x"); ok {
		t.Fatal("an unknown flag parsed as valid")
	}
	if _, ok := parseRegExpFlags("uv"); ok {
		t.Fatal("the u and v pair parsed as valid")
	}
}

// A pattern lowers only when its ECMAScript meaning survives translation to RE2. The
// ordinary character, class, quantifier, group, and alternation core translates and
// compiles, the dot is rewritten to the ECMAScript line-terminator set, and every
// construct RE2 cannot host with the same meaning hands back.
func TestTranslateRegExp(t *testing.T) {
	ok := []struct{ pattern, flags string }{
		{"abc", ""},
		{"a[bc]+d", ""},
		{`\d{2,4}`, ""},
		{"a(b|c)d", ""},
		{"a(?:b|c)d", ""},
		{"foo", "i"},
		{"foo.bar", ""},
		{"foo.bar", "s"},
		{"abc", "m"}, // multiline without an anchor is fine
	}
	for _, c := range ok {
		fl, _ := parseRegExpFlags(c.flags)
		src, got, reason := translateRegExp(c.pattern, fl)
		if !got {
			t.Errorf("translateRegExp(%q, %q) handed back: %s", c.pattern, c.flags, reason)
			continue
		}
		if _, err := regexp.Compile(src); err != nil {
			t.Errorf("translateRegExp(%q, %q) => %q does not compile: %v", c.pattern, c.flags, src, err)
		}
	}

	handback := []struct{ pattern, flags string }{
		{`(a)\1`, ""},      // backreference
		{`(?=foo)`, ""},    // lookahead
		{`(?!foo)`, ""},    // negative lookahead
		{`(?<=foo)`, ""},   // lookbehind
		{`(?<name>x)`, ""}, // named group, a later slice
		{`\p{L}`, ""},      // unicode property escape, a later slice
		{`foo`, "u"},       // unicode mode, a later slice
		{`foo`, "v"},       // unicode-sets mode, a later slice
		{`^foo`, "m"},      // multiline anchor needs the ECMAScript terminator set
	}
	for _, c := range handback {
		fl, _ := parseRegExpFlags(c.flags)
		if _, got, _ := translateRegExp(c.pattern, fl); got {
			t.Errorf("translateRegExp(%q, %q) lowered, want handback", c.pattern, c.flags)
		}
	}
}

// The dot without the s flag excludes exactly ECMAScript's four line terminators, so
// a translated dot matches an ordinary character and not a carriage return, the
// divergence RE2's own dot would get wrong.
func TestTranslateDotLineTerminators(t *testing.T) {
	fl, _ := parseRegExpFlags("")
	src, ok, _ := translateRegExp(".", fl)
	if !ok {
		t.Fatal("the dot pattern handed back")
	}
	re := regexp.MustCompile(src)
	if !re.MatchString("a") {
		t.Error("the translated dot did not match an ordinary character")
	}
	if re.MatchString("\r") {
		t.Error("the translated dot matched a carriage return, an ECMAScript line terminator")
	}
	if re.MatchString("\n") {
		t.Error("the translated dot matched a newline")
	}
}

// NewRegExpLiteral builds a RegExp whose source is the pattern text and whose flags
// are broken out, and an empty pattern reports the "(?:)" source that round-trips
// through the constructor.
func TestNewRegExpLiteral(t *testing.T) {
	re := NewRegExpLiteral("ab+c", "gi")
	if got := re.source.ToGoString(); got != "ab+c" {
		t.Fatalf("source = %q, want ab+c", got)
	}
	if !re.global || !re.ignoreCase {
		t.Fatalf("flags not recorded: %+v", re)
	}
	empty := NewRegExpLiteral("", "")
	if got := empty.source.ToGoString(); got != "(?:)" {
		t.Fatalf("empty source = %q, want (?:)", got)
	}
}

// Source and Flags report the pattern text and the flag run in the specification's
// canonical order d g i m s u v y regardless of the order the flags were written,
// and each single-flag getter reports its own flag.
func TestRegExpAccessors(t *testing.T) {
	re := NewRegExpLiteral("ab+c", "yim")
	if got := re.Source().ToGoString(); got != "ab+c" {
		t.Fatalf("Source() = %q, want ab+c", got)
	}
	if got := re.Flags().ToGoString(); got != "imy" {
		t.Fatalf("Flags() = %q, want imy (canonical order)", got)
	}
	if !re.IgnoreCase() || !re.Multiline() || !re.Sticky() {
		t.Fatalf("single-flag getters wrong: i=%v m=%v y=%v", re.IgnoreCase(), re.Multiline(), re.Sticky())
	}
	if re.Global() || re.DotAll() || re.Unicode() || re.UnicodeSets() || re.HasIndices() {
		t.Fatalf("an unset flag read true: %+v", re)
	}
	all := NewRegExpLiteral("x", "dgs")
	if got := all.Flags().ToGoString(); got != "dgs" {
		t.Fatalf("Flags() = %q, want dgs", got)
	}
	if !all.HasIndices() || !all.Global() || !all.DotAll() {
		t.Fatalf("d/g/s getters wrong: %+v", all)
	}
}

// A non-global exec returns the match array with the whole match at index 0, the
// captures after it, the .index of the match, and the .input it ran against, and
// returns null with no match. A non-global regexp never reads or writes lastIndex.
func TestRegExpExec(t *testing.T) {
	re := NewRegExpLiteral("a(b+)c", "")
	got := re.Exec(FromGoString("xxabbbcyy"))
	if got.IsNull() {
		t.Fatal("exec returned null on a matching input")
	}
	if s := got.GetIndex(0).AsString().ToGoString(); s != "abbbc" {
		t.Errorf("match[0] = %q, want abbbc", s)
	}
	if s := got.GetIndex(1).AsString().ToGoString(); s != "bbb" {
		t.Errorf("match[1] = %q, want bbb", s)
	}
	if idx := got.Get(FromGoString("index")).AsNumber(); idx != 2 {
		t.Errorf("match.index = %v, want 2", idx)
	}
	if in := got.Get(FromGoString("input")).AsString().ToGoString(); in != "xxabbbcyy" {
		t.Errorf("match.input = %q, want xxabbbcyy", in)
	}
	if re.LastIndex() != 0 {
		t.Errorf("a non-global exec wrote lastIndex = %v", re.LastIndex())
	}
	if !re.Exec(FromGoString("nope")).IsNull() {
		t.Error("exec returned a result on a non-matching input")
	}
}

// A non-participating capture group reports undefined at its slot, not the empty
// string, the distinction the match array preserves.
func TestRegExpExecOptionalGroup(t *testing.T) {
	re := NewRegExpLiteral("a(x)?b", "")
	got := re.Exec(FromGoString("ab"))
	if got.IsNull() {
		t.Fatal("exec returned null on a matching input")
	}
	if !got.GetIndex(1).IsUndefined() {
		t.Errorf("an absent group reported %v, want undefined", got.GetIndex(1))
	}
}

// A global exec resumes from lastIndex and advances it past each match, so successive
// calls walk the string and the call after the last match returns null and resets
// lastIndex to zero.
func TestRegExpGlobalLastIndex(t *testing.T) {
	re := NewRegExpLiteral("a", "g")
	starts := []float64{}
	for {
		m := re.Exec(FromGoString("aXaXa"))
		if m.IsNull() {
			break
		}
		starts = append(starts, m.Get(FromGoString("index")).AsNumber())
	}
	if len(starts) != 3 || starts[0] != 0 || starts[1] != 2 || starts[2] != 4 {
		t.Fatalf("global match indices = %v, want [0 2 4]", starts)
	}
	if re.LastIndex() != 0 {
		t.Errorf("lastIndex after the exhausting call = %v, want 0", re.LastIndex())
	}
}

// A sticky regexp matches only at lastIndex: it succeeds when the match begins there
// and fails, resetting lastIndex, when it does not, even though a plain search would
// find the pattern later in the string.
func TestRegExpSticky(t *testing.T) {
	re := NewRegExpLiteral("a", "y")
	re.SetLastIndex(1)
	if !re.Test(FromGoString("babab")) {
		t.Error("a sticky match at 1 missed the 'a' at position 1")
	}
	if re.LastIndex() != 2 {
		t.Errorf("a sticky match advanced lastIndex to %v, want 2", re.LastIndex())
	}
	re2 := NewRegExpLiteral("a", "y")
	re2.SetLastIndex(0)
	if re2.Test(FromGoString("babab")) {
		t.Error("a sticky match at 0 succeeded though position 0 is 'b'")
	}
	if re2.LastIndex() != 0 {
		t.Errorf("a failed sticky match left lastIndex = %v, want 0", re2.LastIndex())
	}
}

// test reports a boolean and shares exec's stateful advance, so a global test walks
// the string across calls the same way exec does.
func TestRegExpTest(t *testing.T) {
	re := NewRegExpLiteral("\\d+", "")
	if !re.Test(FromGoString("abc123")) {
		t.Error("test missed a matching input")
	}
	if re.Test(FromGoString("abc")) {
		t.Error("test matched a non-matching input")
	}
}
