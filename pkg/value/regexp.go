package value

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
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
	// is stored rather than derived. It is a UTF-16 code-unit offset, the same unit
	// .index and String.prototype.length count in, and a Number because a program may
	// assign it any value, which exec then coerces with ToLength on use.
	lastIndex float64

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

// Source returns the pattern text .source reports, the ECMAScript source the
// program wrote (or "(?:)" for the empty pattern), not the RE2 text the match runs
// on. It is a BStr so it flows into the string world unchanged.
func (re *RegExp) Source() BStr { return re.source }

// Flags returns the flag string .flags reports: the flags the regexp carries in the
// canonical order the specification fixes, d g i m s u v y, so two regexps with the
// same flags always report the same string regardless of how they were written.
func (re *RegExp) Flags() BStr {
	var b []byte
	if re.hasIndices {
		b = append(b, 'd')
	}
	if re.global {
		b = append(b, 'g')
	}
	if re.ignoreCase {
		b = append(b, 'i')
	}
	if re.multiline {
		b = append(b, 'm')
	}
	if re.dotAll {
		b = append(b, 's')
	}
	if re.unicode {
		b = append(b, 'u')
	}
	if re.unicodeSet {
		b = append(b, 'v')
	}
	if re.sticky {
		b = append(b, 'y')
	}
	return FromGoString(string(b))
}

// The single-flag accessors report each flag as a boolean, the reads .global,
// .ignoreCase, and the rest make. They mirror the flags string Flags builds, one
// getter per flag, so a program can test one flag without parsing the string.
func (re *RegExp) Global() bool      { return re.global }
func (re *RegExp) IgnoreCase() bool  { return re.ignoreCase }
func (re *RegExp) Multiline() bool   { return re.multiline }
func (re *RegExp) DotAll() bool      { return re.dotAll }
func (re *RegExp) Unicode() bool     { return re.unicode }
func (re *RegExp) UnicodeSets() bool { return re.unicodeSet }
func (re *RegExp) Sticky() bool      { return re.sticky }
func (re *RegExp) HasIndices() bool  { return re.hasIndices }

// LastIndex reports the lastIndex property, the offset a global or sticky match
// resumes from. It is a Number read back exactly as it was last written or last
// advanced, so a program that sets it and reads it sees its own value.
func (re *RegExp) LastIndex() float64 { return re.lastIndex }

// SetLastIndex writes the lastIndex property, the re.lastIndex = n assignment. The
// value is stored as given and only coerced with ToLength when a match reads it, so
// the property read reports the raw assignment the way the specification's data
// property does.
func (re *RegExp) SetLastIndex(v float64) { re.lastIndex = v }

// Exec runs RegExp.prototype.exec (22 §22.2.7.2): it matches the pattern against s
// and returns the match result array on success or null on failure. Under the global
// or sticky flag it starts from lastIndex and advances lastIndex past the match, and
// resets lastIndex to zero on a failed match; a plain regexp ignores lastIndex, never
// writes it, and always searches from the start. The result is a value.Value because
// exec returns an array or null, the RegExpExecArray | null union the checker gives it.
func (re *RegExp) Exec(s BStr) Value {
	m, ok := re.match(s)
	if !ok {
		return Null
	}
	return re.buildResult(s, m)
}

// Test runs RegExp.prototype.test (22 §22.2.7.10), reporting whether the pattern
// matches s. It shares exec's stateful search, so the global and sticky flags advance
// and reset lastIndex the same way; only the return differs, a boolean rather than the
// match array, and no result object is built.
func (re *RegExp) Test(s BStr) bool {
	_, ok := re.match(s)
	return ok
}

// match is the stateful search exec and test share. For a global or sticky regexp it
// reads lastIndex, converts that UTF-16 offset to the byte offset RE2 works in, and
// searches the subject from there; a sticky regexp additionally requires the match to
// begin exactly at that offset. It returns the submatch byte-index pairs in absolute
// coordinates, and updates lastIndex to the UTF-16 offset past the match on success or
// to zero on failure, but only when the global or sticky flag makes lastIndex live.
func (re *RegExp) match(s BStr) ([]int, bool) {
	str := s.ToGoString()
	stateful := re.global || re.sticky
	startByte := 0
	if stateful {
		off, ok := utf16OffsetToByte(str, lastIndexToLength(re.lastIndex))
		if !ok {
			re.lastIndex = 0
			return nil, false
		}
		startByte = off
	}
	loc := re.re.FindStringSubmatchIndex(str[startByte:])
	if loc == nil || (re.sticky && loc[0] != 0) {
		if stateful {
			re.lastIndex = 0
		}
		return nil, false
	}
	// Shift the slice-relative byte indices back into absolute coordinates so the
	// result and lastIndex are computed against the whole subject, not the tail.
	abs := make([]int, len(loc))
	for i, v := range loc {
		if v < 0 {
			abs[i] = -1
		} else {
			abs[i] = v + startByte
		}
	}
	if stateful {
		re.lastIndex = float64(byteToUTF16Offset(str, abs[1]))
	}
	return abs, true
}

// buildResult packs the match into the array RegExp.prototype.exec returns: element
// zero is the whole match, each following element is a capture group's text or
// undefined when the group did not participate, and the array carries the .index of
// the match, the .input it ran against, and the .groups object for named groups. The
// indices arrive in bytes and .index is reported in the UTF-16 code units the language
// counts positions in.
func (re *RegExp) buildResult(s BStr, m []int) Value {
	str := s.ToGoString()
	n := len(m) / 2
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		lo, hi := m[2*i], m[2*i+1]
		if lo < 0 {
			elems[i] = Undefined
		} else {
			elems[i] = StringValue(FromGoString(str[lo:hi]))
		}
	}
	res := NewArrayValue(elems)
	res.Set(FromGoString("index"), Number(float64(byteToUTF16Offset(str, m[0]))))
	res.Set(FromGoString("input"), StringValue(s))
	res.Set(FromGoString("groups"), re.groupsObject(elems))
	return res
}

// groupsObject builds the .groups property of a match result: undefined when the
// pattern has no named groups, else a null-prototype object mapping each group name to
// its captured text, or undefined for a name whose group did not participate. The keys
// are inserted in group-number order, which is the left-to-right order the names appear
// in the pattern, the order RegExpBuiltinExec creates them in. The null prototype is
// what the specification gives the groups object, so a name like "toString" reads its
// captured text and not an inherited method.
func (re *RegExp) groupsObject(elems []Value) Value {
	subNames := re.re.SubexpNames()
	hasNamed := false
	for _, nm := range subNames {
		if nm != "" {
			hasNamed = true
			break
		}
	}
	if !hasNamed {
		return Undefined
	}
	groups := ObjectCreate(Null)
	for i, nm := range subNames {
		if nm == "" {
			continue
		}
		val := Undefined
		if i < len(elems) {
			val = elems[i]
		}
		groups.Set(FromGoString(nm), val)
	}
	return groups
}

// lastIndexToLength coerces a lastIndex Number to the non-negative integer offset
// ToLength yields: a NaN or non-positive value becomes zero, and a fractional value
// truncates toward zero. A value past the subject's length is capped at the array
// length ceiling and then rejected by the offset conversion, the failed-match path
// the specification takes for it. It mirrors toLength but takes a raw float64, since
// lastIndex is already a Number the RegExp stores unboxed.
func lastIndexToLength(v float64) int {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	if v >= maxArrayLength {
		return maxArrayLength
	}
	return int(v)
}

// utf16OffsetToByte converts a UTF-16 code-unit offset into the byte offset of the
// same position in the UTF-8 string, the translation between the unit the language
// counts positions in and the unit RE2 searches in. It reports ok=false when the
// offset is past the end of the string or lands inside a surrogate pair, both of
// which the stateful match treats as a position with no match.
func utf16OffsetToByte(s string, u16 int) (int, bool) {
	if u16 == 0 {
		return 0, true
	}
	count := 0
	for i, r := range s {
		if count == u16 {
			return i, true
		}
		w := utf16.RuneLen(r)
		if w < 0 {
			w = 1
		}
		count += w
	}
	if count == u16 {
		return len(s), true
	}
	return 0, false
}

// byteToUTF16Offset counts the UTF-16 code units in the prefix of s up to the byte
// offset b, the reverse of utf16OffsetToByte, used to report .index and lastIndex in
// the units the language counts.
func byteToUTF16Offset(s string, b int) int {
	if b <= 0 {
		return 0
	}
	if b > len(s) {
		b = len(s)
	}
	count := 0
	for i := 0; i < b; {
		r, size := utf8.DecodeRuneInString(s[i:])
		w := utf16.RuneLen(r)
		if w < 0 {
			w = 1
		}
		count += w
		i += size
	}
	return count
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
// lookbehind) or that later slices own (the u and v flags, unicode property escapes).
// A named capture group and an inline i or s flag modifier translate to RE2's own
// spelling. What remains, the ordinary character, class, quantifier, group, and
// alternation core, maps to RE2 unchanged except for the dot, whose line-terminator
// set differs between the two and is rewritten to the ECMAScript set here, scoped by
// the s flag an inline (?s:...) modifier turns on or off.
func translateRegExp(pattern string, fl regExpFlags) (string, bool, string) {
	if fl.unicode {
		return "", false, "a unicode-mode (u flag) regexp is a later slice"
	}
	if fl.unicodeSet {
		return "", false, "a unicode-sets-mode (v flag) regexp is a later slice"
	}

	var b strings.Builder
	inClass := false
	names := map[string]bool{}
	// dotAll tracks the effective s-flag state per group depth, so a dot is rewritten
	// against the scope it sits in. The base frame is the whole-pattern s flag; each
	// group open pushes a frame (inheriting the enclosing state, or the state an inline
	// (?s:...) or (?-s:...) modifier sets), and each group close pops it. The top of the
	// stack is the effective dot-all where the next dot is rewritten.
	dotAll := []bool{fl.dotAll}
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
			// A named capture group (?<name>...) is ECMAScript's spelling of RE2's
			// (?P<name>...), so it translates by rewriting the prefix and copying the body
			// through, but only when RE2 can host the name: RE2 accepts [A-Za-z0-9_]+ and
			// forbids a repeated name, the two cases regexp.Compile would reject, so a name
			// outside that set or a duplicate hands back rather than emitting a pattern the
			// runtime constructor could not build. Lookbehind and a malformed group fall to
			// heldGroupPrefix below.
			if name, consumed, ok := namedGroupPrefix(rs, i); ok {
				if !validCaptureName(name) {
					return "", false, "a named group whose name RE2 cannot host is a later slice"
				}
				if names[name] {
					return "", false, "a duplicate named group in a regexp is a construct RE2 cannot host"
				}
				names[name] = true
				b.WriteString("(?P<" + name + ">")
				dotAll = append(dotAll, dotAll[len(dotAll)-1])
				i += consumed - 1
				continue
			}
			// An inline flag modifier (?i:...) or (?s:...) is ECMAScript's spelling of a
			// scoped flag change, which RE2 hosts with the same i and s letters, so it
			// passes through and its s scope is tracked for the dot rewrite. An m modifier
			// or the bare (?flags) form hands back.
			if prefix, da, consumed, host, held, reason := parseInlineModifier(rs, i, dotAll[len(dotAll)-1]); host {
				b.WriteString(prefix)
				dotAll = append(dotAll, da)
				i += consumed - 1
				continue
			} else if held {
				return "", false, reason
			}
			// A group prefix names a construct RE2 cannot host (lookahead, lookbehind). A
			// plain capturing group and a non-capturing (?:...) group pass through.
			if kind, held, reason := heldGroupPrefix(rs, i); held {
				return "", false, reason
			} else if kind != "" {
				b.WriteString(kind)
				dotAll = append(dotAll, dotAll[len(dotAll)-1])
				i += len(kind) - 1
				continue
			}
			b.WriteRune(c)
			dotAll = append(dotAll, dotAll[len(dotAll)-1])
		case c == ')':
			// Close the innermost group's scope. A stray ) with no open group leaves the
			// base frame in place; regexp.Compile rejects the unbalanced pattern.
			if len(dotAll) > 1 {
				dotAll = dotAll[:len(dotAll)-1]
			}
			b.WriteRune(c)
		case c == '.':
			// ECMAScript's dot excludes the four line terminators \n \r    ;
			// RE2's dot without the s flag excludes only \n, so a faithful dot is spelled
			// as the explicit negated class. Under the s flag the dot matches every code
			// point including the terminators, which RE2's (?s) dot does exactly. The dot
			// is rewritten against the effective s state of the group it sits in, the top
			// of the scope stack, so an inline (?s:.) or (?-s:.) modifier is honored.
			if dotAll[len(dotAll)-1] {
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
	// A global or sticky regexp resumes its match from lastIndex, which the runtime
	// reaches by slicing the subject at that offset before handing it to RE2. Slicing
	// severs the left context an anchor or a word boundary reads, so ^, $, \b, and \B
	// would mean the wrong thing at a nonzero offset: ^ would match at the slice start
	// rather than only at the string start, and \b would test against a character the
	// slice dropped. A non-global non-sticky regexp always searches from the start, so
	// no slice happens and these assertions stay faithful; only the stateful case hands
	// back, and only when the pattern actually carries one of them.
	if (fl.global || fl.sticky) && patternHasAnchor(pattern) {
		return "", false, "a global or sticky regexp with an anchor or word boundary resumes from a sliced offset RE2 cannot host faithfully, a later slice"
	}
	return src, true, ""
}

// RegExpSourceHasAnchor reports whether a pattern uses a ^ or $ anchor or a \b or \B
// word boundary, the position assertions whose meaning depends on surrounding text.
// The lowerer consults it to keep String.prototype.split off a separator RE2 cannot
// host faithfully: split matches the separator anchored at each offset the way a
// sticky clone does, and slicing the subject at that offset would sever the left
// context such an assertion reads.
func RegExpSourceHasAnchor(pattern string) bool { return patternHasAnchor(pattern) }

// patternHasAnchor reports whether the pattern uses a position assertion whose
// meaning depends on the surrounding text: a bare ^ or $ anchor, or a \b or \B word
// boundary. It respects escapes and character classes, so a \^ escape, a [$] class
// member, or a \b inside a class (which is a backspace, not a boundary) does not
// count. It is the offset-safety test the stateful match paths gate on.
func patternHasAnchor(pattern string) bool {
	rs := []rune(pattern)
	inClass := false
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			if !inClass && i+1 < len(rs) && (rs[i+1] == 'b' || rs[i+1] == 'B') {
				return true
			}
			i++ // skip the escaped character
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
		case c == '^' || c == '$':
			return true
		}
	}
	return false
}

// heldGroupPrefix inspects a group opening at index i. It returns a replacement
// string and no hold for a group that translates (a non-capturing (?:...) passes
// through unchanged, spelled by returning it), held=true with a reason for a group
// RE2 cannot host or a later slice owns, and the empty replacement with held=false
// for a plain capturing group the caller copies. Named groups and hostable inline
// flag modifiers are translated before this runs, so lookahead, lookbehind, and an
// unrecognized group prefix are the held cases here.
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
		// A well-formed named group is translated before heldGroupPrefix runs, so a
		// (?< that reaches here has no closing >, an unterminated named group.
		return "", true, "an unterminated named group in a regexp is a later slice"
	default:
		return "", true, "an unrecognized group prefix in a regexp is a later slice"
	}
}

// parseInlineModifier inspects a group opening at index i and parses an inline flag
// modifier group, (?flags:...) or (?flags-flags:...). ECMAScript's regexp-modifiers
// admit the i, m, and s flags; RE2 shares the i and s spelling and their meaning, so a
// modifier over those two passes through unchanged and its dot-all scope, which the s
// flag sets, is returned for the dot rewrite. base is the enclosing scope's dot-all
// state, which the group inherits before adjusting.
//
// It reports host=true with the RE2 prefix to emit, the group's dot-all state, and the
// runes the prefix spans when the modifier is one RE2 can host. It reports held=true
// with a reason for a modifier RE2 cannot host faithfully: one naming the m flag, whose
// ECMAScript anchor line-terminator set RE2 does not share, or the bare (?flags) form
// with no colon, which applies to the rest of the enclosing group rather than a clean
// nested scope. It reports host=false and held=false when the opening is not a modifier
// at all (a (?:, (?=, (?!, or (?< prefix), which the caller routes to heldGroupPrefix.
func parseInlineModifier(rs []rune, i int, base bool) (prefix string, dotAll bool, consumed int, host, held bool, reason string) {
	if i+2 >= len(rs) || rs[i+1] != '?' {
		return "", false, 0, false, false, ""
	}
	if c := rs[i+2]; c != 'i' && c != 'm' && c != 's' && c != '-' {
		return "", false, 0, false, false, "" // not a modifier opening
	}
	dotAll = base
	sawFlag := false
	neg := false
	j := i + 2
	for ; j < len(rs) && rs[j] != ':'; j++ {
		switch rs[j] {
		case ')':
			return "", false, 0, false, true, "a bare inline flag modifier in a regexp is a later slice"
		case '-':
			if neg {
				return "", false, 0, false, true, "a malformed inline flag modifier in a regexp is a later slice"
			}
			neg = true
		case 'i':
			sawFlag = true
		case 's':
			sawFlag = true
			dotAll = !neg
		case 'm':
			return "", false, 0, false, true, "an inline multiline modifier in a regexp needs the ECMAScript line-terminator set, a later slice"
		default:
			return "", false, 0, false, true, "an inline flag modifier with an unsupported flag in a regexp is a later slice"
		}
	}
	if j >= len(rs) {
		return "", false, 0, false, true, "an unterminated inline flag modifier in a regexp is a later slice"
	}
	if !sawFlag {
		return "", false, 0, false, true, "an empty inline flag modifier in a regexp is a later slice"
	}
	return string(rs[i : j+1]), dotAll, j - i + 1, true, false, ""
}

// namedGroupPrefix inspects a group opening at index i and, when it is a named
// capture group (?<name>, returns the name and the number of runes the whole prefix
// spans, from the ( through the >. It reports ok=false for a lookbehind (?<= or (?<!,
// whose < is not a name, and for an unterminated (?<name with no closing >, both of
// which fall through to heldGroupPrefix. The name is returned untranslated; the caller
// validates it against RE2's accepted set before rewriting the prefix.
func namedGroupPrefix(rs []rune, i int) (name string, consumed int, ok bool) {
	if i+3 >= len(rs) || rs[i+1] != '?' || rs[i+2] != '<' {
		return "", 0, false
	}
	if rs[i+3] == '=' || rs[i+3] == '!' {
		return "", 0, false // a lookbehind, not a named group
	}
	j := i + 3
	for j < len(rs) && rs[j] != '>' {
		j++
	}
	if j >= len(rs) {
		return "", 0, false // unterminated, heldGroupPrefix reports it
	}
	return string(rs[i+3 : j]), j - i + 1, true
}

// validCaptureName reports whether name is a capture name RE2 accepts, the
// [A-Za-z0-9_]+ set Go's regexp/syntax enforces. ECMAScript admits a wider identifier
// set (a leading $ or a Unicode letter), so a name outside RE2's set hands back rather
// than compiling to a program the runtime constructor could not build.
func validCaptureName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		switch {
		case c == '_':
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		default:
			return false
		}
	}
	return true
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
