package value

import (
	"fmt"
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

	// lineNormalize records that this pattern's ^ or $ has to see ECMAScript's line
	// terminators rather than RE2's. RE2 breaks a line only at \n while the language
	// breaks it at \r and the two separators too, so a subject carrying one of those
	// is normalized before the search; see normalizeLineTerminators. It is set only
	// for a multiline pattern that actually uses an anchor and that the translator
	// proved cannot tell the four terminators apart, so normalizing changes no other
	// answer the pattern gives.
	lineNormalize bool

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
		source:        FromGoString(canonicalSource(pattern)),
		re:            prog,
		lineNormalize: multilineAnchored(pattern, fl),

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

// ToStringBStr renders the regexp the way RegExp.prototype.toString does,
// "/" + source + "/" + flags, so String(re), `${re}`, "" + re, and re.toString()
// all read the literal form the program wrote. The source is the .source getter's
// text, already "(?:)" for the empty pattern, and the flags are the canonical run
// Flags builds, so /a/gi stringifies to "/a/gi" and // to "/(?:)/".
func (re *RegExp) ToStringBStr() BStr {
	slash := FromGoString("/")
	return slash.ConcatN(re.source, slash, re.Flags())
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

// A JavaScript regexp matches over UTF-16 code units, but RE2 matches over the runes
// of a UTF-8 string, and the two disagree exactly on the characters that are not a
// single code unit: a supplementary character is one rune but two code units, and a
// lone surrogate is not a scalar value at all. So a subject is transcoded to a "unit
// string" before it is matched, one rune per UTF-16 code unit, so RE2's per-rune
// operators — the dot, a negated class, an anchor — count units the way ECMAScript
// does and a single dot does not swallow a surrogate pair. A code unit in the Basic
// Multilingual Plane is already a valid rune and maps to itself, so an all-BMP subject
// (the overwhelming common case) is byte-identical to its UTF-8 form and takes the
// unchanged path; a surrogate code unit, which cannot be a Go rune, maps into a
// private-use block so it survives as one rune, and a supplementary character becomes
// the two surrogate runes it is spelled with, so the dot no longer matches it whole.
const surrogateRuneBase = 0xF0000
const surrogateRuneEnd = surrogateRuneBase + 0x7FF

// unitToRune maps a UTF-16 code unit to the rune that stands for it in a unit string:
// a surrogate goes to the private-use block, every other unit is its own rune.
func unitToRune(u uint16) rune {
	if u >= 0xD800 && u <= 0xDFFF {
		return surrogateRuneBase + rune(u-0xD800)
	}
	return rune(u)
}

// isMappedSurrogate reports whether a rune of a unit string stands for a surrogate
// code unit, the runes re2Text must map back rather than copy.
func isMappedSurrogate(r rune) bool { return r >= surrogateRuneBase && r <= surrogateRuneEnd }

// needsUnitForm reports whether s must be transcoded before matching: it holds a
// supplementary character (two code units) or a lone surrogate, the only cases where a
// rune of the UTF-8 form is not exactly one UTF-16 code unit. An all-BMP string does
// not, so it keeps the UTF-8 fast path with no allocation.
func needsUnitForm(s BStr) bool {
	s = s.flat()
	if s.utf16 != nil {
		return true // the code-unit view exists only for a lone surrogate
	}
	for _, r := range s.utf8 {
		if r > 0xFFFF {
			return true
		}
	}
	return false
}

// re2Subject returns the string RE2 matches s against: the plain UTF-8 form when s is
// all-BMP, or the unit-transcoded form when s holds a supplementary character or lone
// surrogate, so each rune of the result is exactly one UTF-16 code unit.
func re2Subject(s BStr) string {
	if !needsUnitForm(s) {
		return s.ToGoString()
	}
	units := s.units()
	var b strings.Builder
	b.Grow(len(units) + len(units)/2)
	for _, u := range units {
		b.WriteRune(unitToRune(u))
	}
	return b.String()
}

// re2Unit converts a byte offset in a re2Subject string to the UTF-16 code-unit offset
// the language reports positions in. Every rune of a unit string is one code unit, so
// the offset is the rune count of the prefix.
func re2Unit(str string, b int) int {
	if b <= 0 {
		return 0
	}
	if b > len(str) {
		b = len(str)
	}
	return utf8.RuneCountInString(str[:b])
}

// re2Byte converts a UTF-16 code-unit offset to the byte offset of that position in a
// re2Subject string, advancing one rune per unit. It reports ok=false for an offset
// past the end, the position the stateful match treats as no match.
func re2Byte(str string, u int) (int, bool) {
	if u <= 0 {
		return 0, true
	}
	count := 0
	for i := range str {
		if count == u {
			return i, true
		}
		count++
	}
	if count == u {
		return len(str), true
	}
	return 0, false
}

// re2Text reconstructs the string a byte range of a re2Subject denotes, undoing the
// surrogate mapping so a matched supplementary character reads back as its two code
// units and a mapped surrogate reads back as itself. A range with no mapped surrogate
// and no supplementary rune keeps the UTF-8 fast path; otherwise the units are rebuilt.
func re2Text(str string, lo, hi int) BStr {
	sub := str[lo:hi]
	if !strings.ContainsFunc(sub, func(r rune) bool { return isMappedSurrogate(r) || r > 0xFFFF }) {
		return FromGoString(sub)
	}
	var units []uint16
	for _, r := range sub {
		switch {
		case isMappedSurrogate(r):
			units = append(units, uint16(0xD800+(r-surrogateRuneBase)))
		case r > 0xFFFF:
			h, l := utf16.EncodeRune(r)
			units = append(units, uint16(h), uint16(l))
		default:
			units = append(units, uint16(r))
		}
	}
	return FromUTF16(units)
}

// re2Whole maps a fully assembled transcoded string back into a BStr, undoing the
// unit-rune transcoding over its whole length. The string-side methods build a result
// by splicing transcoded subject slices with transcoded replacement text, so the whole
// thing is in the unit-rune domain and is mapped back in one pass at the end.
func re2Whole(str string) BStr { return re2Text(str, 0, len(str)) }

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
	ns := re.searchText(re2Subject(s))
	stateful := re.global || re.sticky
	startByte := 0
	if stateful {
		off, ok := re2Byte(ns.text, lastIndexToLength(re.lastIndex))
		if !ok {
			re.lastIndex = 0
			return nil, false
		}
		startByte = off
	}
	loc := re.re.FindStringSubmatchIndex(ns.text[startByte:])
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
	// lastIndex counts UTF-16 units, which the normalization preserves one for one, so
	// it reads off the searched text; the returned byte indices are mapped back to the
	// subject the caller holds, whose separators are three bytes rather than one.
	if stateful {
		re.lastIndex = float64(re2Unit(ns.text, abs[1]))
	}
	return ns.subjectIndices(abs), true
}

// lineNormalized is the text a multiline search runs against together with what it
// takes to read a position in it back as a position in the subject. text is the
// subject with every ECMAScript line terminator rewritten to \n, the one break RE2
// knows, so ^ and $ land where the language says they do. shifts holds the byte
// offsets in text where a three-byte separator collapsed into that one byte, in
// order, which is the only place the two coordinate systems drift apart.
//
// A regexp that needs no normalization, which is nearly all of them, carries the
// subject itself and no shifts, and every mapping below is then the identity.
type lineNormalized struct {
	text   string
	shifts []int
}

// subjectByte reads a byte offset in the searched text back as a byte offset in the
// subject. A negative offset is the "group did not participate" marker and passes
// through.
func (n lineNormalized) subjectByte(off int) int {
	if off < 0 || len(n.shifts) == 0 {
		return off
	}
	extra := 0
	for _, p := range n.shifts {
		if p >= off {
			break
		}
		extra += 2
	}
	return off + extra
}

// subjectIndices maps a whole submatch index slice back into subject coordinates.
func (n lineNormalized) subjectIndices(loc []int) []int {
	if loc == nil || len(n.shifts) == 0 {
		return loc
	}
	out := make([]int, len(loc))
	for i, v := range loc {
		out[i] = n.subjectByte(v)
	}
	return out
}

// searchText prepares the subject for this regexp's search. Only an anchored
// multiline pattern sees anything but the subject itself, and then only when the
// subject actually carries a terminator RE2 does not break a line at: \r and the two
// separators become \n, so RE2's line-oriented ^ and $ answer what ECMAScript's do.
//
// The rewrite is invisible to the rest of the match because the translator admitted
// the pattern only after proving it cannot tell one terminator from another, so no
// atom in it can see the difference, and because every replaced character is one
// UTF-16 unit before and after, so .index, lastIndex, and every other position the
// language reports are unchanged. Only byte offsets move, which subjectByte undoes.
func (re *RegExp) searchText(str string) lineNormalized {
	if !re.lineNormalize || !strings.ContainsAny(str, "\r\u2028\u2029") {
		return lineNormalized{text: str}
	}
	var b strings.Builder
	b.Grow(len(str))
	var shifts []int
	for _, r := range str {
		switch r {
		case '\r':
			b.WriteByte('\n')
		case '\u2028', '\u2029':
			shifts = append(shifts, b.Len())
			b.WriteByte('\n')
		default:
			b.WriteRune(r)
		}
	}
	return lineNormalized{text: b.String(), shifts: shifts}
}

// buildResult packs the match into the array RegExp.prototype.exec returns: element
// zero is the whole match, each following element is a capture group's text or
// undefined when the group did not participate, and the array carries the .index of
// the match, the .input it ran against, and the .groups object for named groups. The
// indices arrive in bytes and .index is reported in the UTF-16 code units the language
// counts positions in.
func (re *RegExp) buildResult(s BStr, m []int) Value {
	str := re2Subject(s)
	n := len(m) / 2
	elems := make([]Value, n)
	for i := 0; i < n; i++ {
		lo, hi := m[2*i], m[2*i+1]
		if lo < 0 {
			elems[i] = Undefined
		} else {
			elems[i] = StringValue(re2Text(str, lo, hi))
		}
	}
	res := NewArrayValue(elems)
	res.Set(FromGoString("index"), Number(float64(re2Unit(str, m[0]))))
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
// jsWhitespaceClass is RE2's spelling of ECMAScript's \s: tab, newline, vertical tab,
// form feed, carriage return, U+FEFF, and every Unicode space separator (\p{Z} = Zs+Zl+Zp,
// which covers space, NBSP, U+1680, U+2000-200A, U+2028, U+2029, U+202F, U+205F, U+3000).
// jsNonWhitespaceClass is its negation, RE2's spelling of \S.
const (
	jsWhitespaceClass    = `[\t\n\x0b\f\r\x{feff}\p{Z}]`
	jsNonWhitespaceClass = `[^\t\n\x0b\f\r\x{feff}\p{Z}]`
)

// writeRE2Literal writes a literal character to the RE2 pattern. A supplementary
// character is not one UTF-16 code unit, so in a unit string it is spelled with its two
// surrogate runes; the pattern splits it the same way so it matches the transcoded
// subject, and inside a character class the two runes match either half the way a
// non-unicode-mode regexp treats a raw astral character in a class. A BMP character is
// written unchanged, which is what the pattern needs whether it is a literal or one of
// the RE2 metacharacters (+ * ? { | ^ $) the caller routes here.
func writeRE2Literal(b *strings.Builder, c rune) {
	if c > 0xFFFF {
		h, l := utf16.EncodeRune(c)
		fmt.Fprintf(b, `\x{%x}\x{%x}`, unitToRune(uint16(h)), unitToRune(uint16(l)))
		return
	}
	b.WriteRune(c)
}

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
			// ECMAScript's \s and \S span the full Unicode WhiteSpace set (tab, the line
			// terminators, the space separators \p{Z}, and the zero-width no-break space
			// U+FEFF), but RE2's \s is ASCII-only, so a bare \s / \S would wrongly reject
			// or accept characters like NBSP or U+3000. Outside a character class the escape
			// is rewritten to the explicit Unicode class (and its negation) that matches JS.
			// Inside a class the escape is left verbatim: RE2 cannot host an inline negated
			// property, and splicing would change the common [\s\S] match-any idiom, so the
			// pre-existing ASCII behavior stands there rather than risk a regression.
			if !inClass && (n == 's' || n == 'S') {
				if n == 's' {
					b.WriteString(jsWhitespaceClass)
				} else {
					b.WriteString(jsNonWhitespaceClass)
				}
				i++
				continue
			}
			// A supplementary character after a backslash is an identity escape yielding the
			// character itself (\X is X for a non-special X), so it is emitted as its two
			// surrogate units, without the backslash, to match the transcoded subject. A BMP
			// escape is copied verbatim so its RE2 meaning carries through.
			if n > 0xFFFF {
				writeRE2Literal(&b, n)
			} else {
				b.WriteRune(c)
				b.WriteRune(n)
			}
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
			writeRE2Literal(&b, c)
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
			writeRE2Literal(&b, c)
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
		// \r and the two separators too, so an anchored multiline pattern would disagree
		// on a subject carrying one of those. The match normalizes such a subject instead
		// (normalizeLineTerminators), which is faithful only for a pattern that cannot
		// tell the four terminators apart; one that can still hands back.
		if multilineAnchored(pattern, fl) && patternTellsTerminatorsApart(pattern) {
			return "", false, "a multiline regexp that tells the line terminators apart is a later slice"
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
	// A final trial compile is the honest backstop: the rewrites above translate the
	// constructs bento models, but a scoped inline modifier RE2 spells differently (an
	// empty-remove group like (?s-:...) among them) can still leave a source RE2 rejects.
	// Returning ok=true for it would emit a NewRegExpLiteral that throws at runtime; the
	// zero-fail invariant forbids that, so a source RE2 cannot compile hands back instead.
	if _, err := regexp.Compile(src); err != nil {
		return "", false, "a regexp whose translation RE2 cannot compile is a later slice"
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

// lineTerminators is ECMAScript's LineTerminator set (12 §12.3): the two ASCII
// breaks and the two Unicode separators. RE2 breaks a line only at the first of
// them, which is the whole of the multiline gap the normalization closes.
var lineTerminators = []rune{'\n', '\r', '\u2028', '\u2029'}

// isLineTerminator reports whether r is one of them.
func isLineTerminator(r rune) bool {
	for _, t := range lineTerminators {
		if r == t {
			return true
		}
	}
	return false
}

// multilineAnchored reports whether a pattern's line boundaries are observable: it
// carries the m flag and uses a bare ^ or $ somewhere. Without the flag the anchors
// mean start and end of the whole subject, which RE2 spells the same way, and
// without an anchor the flag changes nothing a match can see. It is the one
// condition the terminator normalization turns on for, so the translator's guard
// and the constructor's flag read it from here rather than each spelling it.
func multilineAnchored(pattern string, fl regExpFlags) bool {
	return fl.multiline && strings.ContainsAny(stripEscapesAndClasses(pattern), "^$")
}

// patternTellsTerminatorsApart reports whether the pattern can distinguish one line
// terminator from another, which is what decides whether normalizing them to \n is
// faithful. Rewriting a subject's \r to \n is invisible to a pattern that either
// matches all four terminators at a position or matches none of them, and visible to
// one that matches some: /\r/ would stop finding what it was written to find, and
// /[^\n]/ would start rejecting what it used to accept.
//
// The scan is syntactic and errs toward true. A terminator named outright, as a
// literal character or through a \n, \r, \xHH, \uHHHH, \u{H...} or \cX escape,
// counts. Inside a class, a range counts when the four terminators fall on both
// sides of it, and \s or \S counts because a class keeps RE2's ASCII reading of
// those, which holds \n and \r but not the two separators. The escapes with a
// uniform answer, \d \D \w \W and \s \S outside a class (rewritten to the explicit
// Unicode classes above), say nothing about which terminator a subject holds and so
// do not count.
func patternTellsTerminatorsApart(pattern string) bool {
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '\\':
			if i+1 >= len(rs) {
				return true // a trailing backslash hands back on its own anyway
			}
			r, consumed, ok := escapedCodePoint(rs, i)
			if !ok {
				i++ // an escape naming a class or an assertion, not a character
				continue
			}
			if isLineTerminator(r) {
				return true
			}
			i += consumed - 1
		case '[':
			apart, consumed := classTellsTerminatorsApart(rs, i)
			if apart {
				return true
			}
			i += consumed - 1
		default:
			if isLineTerminator(c) {
				return true
			}
		}
	}
	return false
}

// classTellsTerminatorsApart reads the character class opening at i and reports
// whether its members hold some line terminators and not others, along with the runes
// it spans. Negation is not asked about: flipping membership keeps a uniform class
// uniform. A range counts when the terminators fall on both sides of it, and \s or \S
// counts because a class keeps RE2's ASCII reading of them, which holds \n and \r but
// neither separator. An unterminated class reads as telling them apart, which costs
// nothing since it hands back on its own.
func classTellsTerminatorsApart(rs []rune, i int) (bool, int) {
	j := i + 1
	if j < len(rs) && rs[j] == '^' {
		j++
	}
	for ; j < len(rs); j++ {
		c := rs[j]
		if c == ']' {
			return false, j - i + 1
		}
		lo, consumed := c, 1
		if c == '\\' {
			if j+1 >= len(rs) {
				return true, 0
			}
			if rs[j+1] == 's' || rs[j+1] == 'S' {
				return true, 0
			}
			r, n, ok := escapedCodePoint(rs, j)
			if !ok {
				j++ // \d, \w and the rest answer the same for all four
				continue
			}
			lo, consumed = r, n
		}
		if isLineTerminator(lo) {
			return true, 0
		}
		if j+consumed+1 < len(rs) && rs[j+consumed] == '-' && rs[j+consumed+1] != ']' {
			hi, n, ok := classRangeEnd(rs, j+consumed+1)
			if !ok {
				return true, 0
			}
			if splitsTerminators(lo, hi) {
				return true, 0
			}
			j = j + consumed + n
			continue
		}
		j += consumed - 1
	}
	return true, 0
}

// escapedCodePoint reads the character an escape at index i denotes, reporting
// ok=false for an escape that names a class (\d, \s), an assertion (\b), or a
// backreference, none of which is a single character. consumed counts the runes the
// escape spans, the backslash included.
func escapedCodePoint(rs []rune, i int) (r rune, consumed int, ok bool) {
	n := rs[i+1]
	switch n {
	case 'n':
		return '\n', 2, true
	case 'r':
		return '\r', 2, true
	case 't':
		return '\t', 2, true
	case 'f':
		return '\f', 2, true
	case 'v':
		return '\v', 2, true
	case '0':
		return 0, 2, true
	case 'x':
		if v, okHex := hexRunes(rs, i+2, 2); okHex {
			return v, 4, true
		}
		return 0, 0, false
	case 'u':
		if i+2 < len(rs) && rs[i+2] == '{' {
			for j := i + 3; j < len(rs); j++ {
				if rs[j] == '}' {
					if v, okHex := hexRunes(rs, i+3, j-(i+3)); okHex {
						return v, j - i + 1, true
					}
					return 0, 0, false
				}
			}
			return 0, 0, false
		}
		if v, okHex := hexRunes(rs, i+2, 4); okHex {
			return v, 6, true
		}
		return 0, 0, false
	case 'c':
		// \cX is the control character X mod 32, so \cJ is \n and \cM is \r.
		if i+2 < len(rs) {
			u := rs[i+2]
			if (u >= 'a' && u <= 'z') || (u >= 'A' && u <= 'Z') {
				return u % 32, 3, true
			}
		}
		return 0, 0, false
	case 'd', 'D', 's', 'S', 'w', 'W', 'b', 'B', 'p', 'P', 'k':
		return 0, 0, false
	}
	if n >= '1' && n <= '9' {
		return 0, 0, false // a backreference, which hands back on its own
	}
	return n, 2, true // an identity escape, the character itself
}

// classRangeEnd reads the upper endpoint of a character-class range starting at i,
// which is either an escape or a plain character.
func classRangeEnd(rs []rune, i int) (r rune, consumed int, ok bool) {
	if rs[i] != '\\' {
		return rs[i], 1, true
	}
	if i+1 >= len(rs) {
		return 0, 0, false
	}
	return escapedCodePoint(rs, i)
}

// hexRunes reads n hex digits starting at i as a code point.
func hexRunes(rs []rune, i, n int) (rune, bool) {
	if n <= 0 || i+n > len(rs) {
		return 0, false
	}
	var v rune
	for _, c := range rs[i : i+n] {
		switch {
		case c >= '0' && c <= '9':
			v = v*16 + (c - '0')
		case c >= 'a' && c <= 'f':
			v = v*16 + (c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v = v*16 + (c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return v, true
}

// splitsTerminators reports whether the inclusive range lo..hi holds some of the line
// terminators and not others, the shape that makes normalizing them visible.
func splitsTerminators(lo, hi rune) bool {
	in := 0
	for _, t := range lineTerminators {
		if t >= lo && t <= hi {
			in++
		}
	}
	return in != 0 && in != len(lineTerminators)
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
