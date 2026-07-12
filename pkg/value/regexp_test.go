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
