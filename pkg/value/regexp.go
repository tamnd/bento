package value

import (
	"regexp"
	"strings"
)

// RegExp is bento's runtime representation of a JavaScript RegExp (22 §22.2). A
// regexp pairs a pattern with a flag set and matches it against a string; bento
// hosts the match on Go's regexp package (RE2) rather than a from-scratch
// backtracking engine, so a pattern lowers only when its ECMAScript semantics
// coincide with what RE2 computes. The lowerer proves that coincidence at compile
// time through TranslateRegExp, the same function this constructor runs, so a
// literal or a constant-pattern constructor that reached here is known to compile;
// a pattern RE2 cannot host faithfully never becomes a RegExp, it hands back at
// lowering instead.
//
// The object carries the original source and the flag set for the source and flags
// accessors, the compiled RE2 program for exec and test, and the lastIndex the
// global and sticky flags advance across successive matches. Source and flags are
// the ECMAScript text the program wrote, not the translated RE2 text, since that is
// what .source and .flags must report.
type RegExp struct {
	source BStr           // the original ECMAScript pattern, what .source reports
	re     *regexp.Regexp // the compiled RE2 program over the translated pattern

	// lastIndex is the index exec and test resume from under the global or sticky
	// flag, and reset to zero on a failed match; it is observable and writable, so it
	// is stored rather than derived.
	lastIndex int

	// The flag set, each flag broken out so a match path reads a bool rather than
	// re-scanning the flags string. The canonical flags string is rebuilt from these
	// in the order the specification fixes.
	global     bool // g: exec/test advance lastIndex and iterate all matches
	ignoreCase bool // i: case-insensitive match
	multiline  bool // m: ^ and $ match at line boundaries
	dotAll     bool // s: . matches a line terminator too
	unicode    bool // u: Unicode mode (code-point semantics)
	unicodeSet bool // v: Unicode-sets mode (set notation and string properties)
	sticky     bool // y: match anchored at lastIndex
	hasIndices bool // d: exec result carries match indices
}

// regExpFlags is the parsed flag set, the shape TranslateRegExp reads so its
// substitutions honor the flags a pattern was written with. It is the same set the
// RegExp object breaks out, split off so the translator and the constructor share
// one parse.
type regExpFlags struct {
	global, ignoreCase, multiline, dotAll, unicode, unicodeSet, sticky, hasIndices bool
}

// parseRegExpFlags reads the flag characters of a regexp into the parsed set,
// reporting ok=false on a character that is not a flag or a flag that repeats, the
// two cases the specification rejects as a SyntaxError. The order the characters
// appear in does not matter; the canonical order is imposed when the flags string
// is read back.
func parseRegExpFlags(flags string) (regExpFlags, bool) {
	var fl regExpFlags
	seen := map[rune]bool{}
	for _, c := range flags {
		if seen[c] {
			return regExpFlags{}, false
		}
		seen[c] = true
		switch c {
		case 'g':
			fl.global = true
		case 'i':
			fl.ignoreCase = true
		case 'm':
			fl.multiline = true
		case 's':
			fl.dotAll = true
		case 'u':
			fl.unicode = true
		case 'v':
			fl.unicodeSet = true
		case 'y':
			fl.sticky = true
		case 'd':
			fl.hasIndices = true
		default:
			return regExpFlags{}, false
		}
	}
	// u and v are mutually exclusive; a pattern that names both is a SyntaxError.
	if fl.unicode && fl.unicodeSet {
		return regExpFlags{}, false
	}
	return fl, true
}

// NewRegExpLiteral builds the RegExp a regexp literal or a constant-pattern
// constructor lowers to. The pattern and flags are the ECMAScript text the program
// wrote; the lowerer already ran TranslateRegExp over them and lowered only on
// success, so the translate-and-compile here cannot fail on a well-formed input.
// It still reports a SyntaxError through Throw on the impossible failure rather
// than panicking, so a bug in the compile-time gate surfaces as a language error
// and never a Go crash.
func NewRegExpLiteral(pattern, flags string) *RegExp {
	fl, ok := parseRegExpFlags(flags)
	if !ok {
		Throw(NewSyntaxError(FromGoString("Invalid regular expression flags")))
	}
	re2, ok, _ := translateRegExp(pattern, fl)
	if !ok {
		Throw(NewSyntaxError(FromGoString("Invalid regular expression: /" + pattern + "/")))
	}
	prog, err := regexp.Compile(re2)
	if err != nil {
		Throw(NewSyntaxError(FromGoString("Invalid regular expression: /" + pattern + "/")))
	}
	return &RegExp{
		source:     FromGoString(canonicalSource(pattern)),
		re:         prog,
		global:     fl.global,
		ignoreCase: fl.ignoreCase,
		multiline:  fl.multiline,
		dotAll:     fl.dotAll,
		unicode:    fl.unicode,
		unicodeSet: fl.unicodeSet,
		sticky:     fl.sticky,
		hasIndices: fl.hasIndices,
	}
}

// canonicalSource returns the text .source reports for a pattern. An empty pattern
// reads back as "(?:)", the specification's non-capturing empty group, so the
// source is always a valid pattern that round-trips through the RegExp constructor;
// every other pattern reports its own text.
func canonicalSource(pattern string) string {
	if pattern == "" {
		return "(?:)"
	}
	return pattern
}

// TranslateRegExpSource is the string-in, string-out gate the lowerer calls at
// compile time to decide whether a pattern and flag pair lowers. It parses the flag
// text and runs the same translation the runtime constructor runs, so a pattern
// lowers exactly when NewRegExpLiteral would build it. It reports the translated RE2
// source on success and ok=false with a reason otherwise, including an invalid flag
// set, which the lowerer surfaces as its handback reason.
func TranslateRegExpSource(pattern, flags string) (re2 string, ok bool, reason string) {
	fl, ok := parseRegExpFlags(flags)
	if !ok {
		return "", false, "a regexp with an invalid flag set is a later slice"
	}
	return translateRegExp(pattern, fl)
}

// translateRegExp converts an ECMAScript pattern to the equivalent RE2 pattern,
// reporting ok=false with a reason when the pattern uses a construct RE2 cannot host
// with the same meaning. It is the single gate both the lowerer and the runtime
// constructor consult, so a pattern lowers exactly when the runtime can build it,
// and the honest handbacks below are the ceiling the RE2 host imposes.
//
// The conservative rule is to translate only what is provably faithful and hand back
// the rest: a mistranslation would run and silently disagree with JavaScript, which
// the zero-fail invariant forbids, whereas a handback is safe. The constructs held
// back here are the ones RE2 does not support at all (backreferences, lookahead,
// lookbehind) or that later slices own (named groups, the u and v flags, unicode
// property escapes, inline flag modifiers). What remains, the ordinary character,
// class, quantifier, group, and alternation core, maps to RE2 unchanged except for
// the dot, whose line-terminator set differs between the two and is rewritten to the
// ECMAScript set here.
func translateRegExp(pattern string, fl regExpFlags) (string, bool, string) {
	if fl.unicode {
		return "", false, "a unicode-mode (u flag) regexp is a later slice"
	}
	if fl.unicodeSet {
		return "", false, "a unicode-sets-mode (v flag) regexp is a later slice"
	}

	var b strings.Builder
	inClass := false
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			// An escape is two characters; the escaped character is copied verbatim so
			// its RE2 meaning matches. A backreference and a named-group escape have no
			// RE2 equivalent, and a unicode property escape is a later slice, so each
			// hands back rather than losing its meaning.
			if i+1 >= len(rs) {
				return "", false, "a trailing backslash in a regexp is a later slice"
			}
			n := rs[i+1]
			if n >= '1' && n <= '9' {
				return "", false, "a backreference in a regexp is a construct RE2 cannot host"
			}
			if n == 'k' {
				return "", false, "a named backreference in a regexp is a construct RE2 cannot host"
			}
			if n == 'p' || n == 'P' {
				return "", false, "a unicode property escape in a regexp is a later slice"
			}
			b.WriteRune(c)
			b.WriteRune(n)
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
			b.WriteRune(c)
		case c == '[':
			inClass = true
			b.WriteRune(c)
		case c == '(':
			// A group prefix names a construct RE2 either cannot host (lookahead,
			// lookbehind) or a later slice owns (a named group, an inline flag modifier).
			// A plain capturing group and a non-capturing (?:...) group pass through.
			if kind, held, reason := heldGroupPrefix(rs, i); held {
				return "", false, reason
			} else if kind != "" {
				b.WriteString(kind)
				i += len(kind) - 1
				continue
			}
			b.WriteRune(c)
		case c == '.':
			// ECMAScript's dot excludes the four line terminators \n \r    ;
			// RE2's dot without the s flag excludes only \n, so a faithful dot is spelled
			// as the explicit negated class. Under the s flag the dot matches every code
			// point including the terminators, which RE2's (?s) dot does exactly.
			if fl.dotAll {
				b.WriteString("(?s:.)")
			} else {
				b.WriteString(`[^\n\r\x{2028}\x{2029}]`)
			}
		default:
			b.WriteRune(c)
		}
	}
	if inClass {
		return "", false, "an unterminated character class in a regexp is a later slice"
	}

	src := b.String()
	prefix := ""
	if fl.ignoreCase {
		prefix += "i"
	}
	if fl.multiline {
		// RE2's multiline flag treats only \n as a line boundary while ECMAScript treats
		// \r and the two line separators too, so a multiline pattern that uses ^ or $
		// would disagree; it hands back until that gap is closed.
		if strings.ContainsAny(stripEscapesAndClasses(pattern), "^$") {
			return "", false, "a multiline regexp using ^ or $ needs the ECMAScript line-terminator set, a later slice"
		}
		prefix += "m"
	}
	if prefix != "" {
		src = "(?" + prefix + ")" + src
	}
	return src, true, ""
}

// heldGroupPrefix inspects a group opening at index i. It returns a replacement
// string and no hold for a group that translates (a non-capturing (?:...) passes
// through unchanged, spelled by returning it), held=true with a reason for a group
// RE2 cannot host or a later slice owns, and the empty replacement with held=false
// for a plain capturing group the caller copies. Named groups, lookahead,
// lookbehind, and inline flag modifiers are the held cases.
func heldGroupPrefix(rs []rune, i int) (replacement string, held bool, reason string) {
	if i+1 >= len(rs) || rs[i+1] != '?' {
		return "", false, "" // a plain capturing group
	}
	if i+2 >= len(rs) {
		return "", true, "an unterminated group in a regexp is a later slice"
	}
	switch rs[i+2] {
	case ':':
		return "(?:", false, "" // non-capturing group, identical in RE2
	case '=', '!':
		return "", true, "a lookahead in a regexp is a construct RE2 cannot host"
	case '<':
		if i+3 < len(rs) && (rs[i+3] == '=' || rs[i+3] == '!') {
			return "", true, "a lookbehind in a regexp is a construct RE2 cannot host"
		}
		return "", true, "a named capture group in a regexp is a later slice"
	default:
		return "", true, "an inline flag modifier in a regexp is a later slice"
	}
}

// stripEscapesAndClasses returns the pattern with its escaped characters and
// character-class bodies removed, so a scan for a bare ^ or $ anchor does not
// mistake a \^ escape or a [$] class member for one. It is a coarse filter used
// only to decide whether the multiline gap above applies.
func stripEscapesAndClasses(pattern string) string {
	var b strings.Builder
	rs := []rune(pattern)
	inClass := false
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			i++ // drop the escape and its escaped character
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
