package value

import (
	"strings"
	"unicode/utf8"
)

// This file lowers the String.prototype methods that take a regexp: match, search,
// replace, replaceAll, and split. Each is the string-side spelling of a RegExp
// method the specification delegates to (String.prototype.match calls
// regexp[Symbol.match], and so on), so it runs on the same RE2 program and the same
// UTF-16 offset accounting exec and test use. The subject string is the argument and
// the RegExp is the receiver, exactly the delegation the specification defines. Only
// a string replacement is handled here; a function replacement, which runs user code
// per match, is a later slice and hands back at lowering.

// Search runs RegExp.prototype[Symbol.search] (22 §22.2.7.9): it reports the UTF-16
// index of the first match or -1, and neither reads nor writes lastIndex, so a
// global or sticky regexp searches from the start the same as a plain one. A sticky
// regexp still matches only at position zero, the search-from-zero the specification
// fixes, so a later match does not count.
func (re *RegExp) Search(s BStr) float64 {
	str := s.ToGoString()
	loc := re.re.FindStringIndex(str)
	if loc == nil || (re.sticky && loc[0] != 0) {
		return -1
	}
	return float64(byteToUTF16Offset(str, loc[0]))
}

// MatchStr runs String.prototype.match (22 §22.1.3.14). A non-global regexp returns
// exec's result, the match array or null. A global regexp resets lastIndex to zero,
// walks every match, and returns an array of the matched substrings (element zero of
// each match), or null when there is no match; an empty match advances one code unit
// so the walk terminates, the AdvanceStringIndex the specification applies.
func (re *RegExp) MatchStr(s BStr) Value {
	if !re.global {
		return re.Exec(s)
	}
	re.lastIndex = 0
	str := s.ToGoString()
	var elems []Value
	for {
		m, ok := re.match(s)
		if !ok {
			break
		}
		elems = append(elems, StringValue(FromGoString(str[m[0]:m[1]])))
		if m[0] == m[1] {
			re.lastIndex++
		}
	}
	if len(elems) == 0 {
		return Null
	}
	return NewArrayValue(elems)
}

// ReplaceStr runs String.prototype.replace and replaceAll with a string replacement
// (22 §22.1.3.19). A non-global regexp replaces the first match; a global regexp
// resets lastIndex and replaces every match, advancing one code unit past an empty
// match so the walk terminates. The replacement template expands the ECMAScript
// substitution patterns $$, $&, $`, $', and $n, so a captured group flows into the
// result the same way the engine substitutes it.
func (re *RegExp) ReplaceStr(s, repl BStr) BStr {
	str := s.ToGoString()
	tmpl := repl.ToGoString()
	if !re.global {
		loc := re.re.FindStringSubmatchIndex(str)
		if loc == nil || (re.sticky && loc[0] != 0) {
			return s
		}
		var b strings.Builder
		b.WriteString(str[:loc[0]])
		b.WriteString(expandReplacement(str, loc, tmpl))
		b.WriteString(str[loc[1]:])
		return FromGoString(b.String())
	}
	re.lastIndex = 0
	var b strings.Builder
	last := 0
	for {
		m, ok := re.match(s)
		if !ok {
			break
		}
		b.WriteString(str[last:m[0]])
		b.WriteString(expandReplacement(str, m, tmpl))
		last = m[1]
		if m[0] == m[1] {
			re.lastIndex++
		}
	}
	b.WriteString(str[last:])
	return FromGoString(b.String())
}

// ReplaceAllStr runs String.prototype.replaceAll (22 §22.1.3.20) with a string
// replacement. replaceAll requires a global regexp and throws a TypeError otherwise,
// the one check that separates it from replace; with the global flag present it
// replaces every match exactly as a global replace does.
func (re *RegExp) ReplaceAllStr(s, repl BStr) BStr {
	if !re.global {
		Throw(NewTypeError(FromGoString("replaceAll must be called with a global RegExp")))
	}
	return re.ReplaceStr(s, repl)
}

// SplitStr runs String.prototype.split with a regexp separator (22 §22.2.7.11). It
// walks the subject, matching the separator anchored at each position the way the
// specification's sticky splitter clone does, cutting the text between matches into
// the result and appending each capture group of the separator after each cut. The
// limit caps the result length, an undefined limit meaning no cap; a zero limit
// yields the empty array, and the empty subject yields [""] unless the separator
// matches there. The anchored match reads no severed left context because the
// lowerer admits split only for a separator with no anchor or word boundary.
func (re *RegExp) SplitStr(s BStr, limited bool, limit float64) Value {
	str := s.ToGoString()
	lim := int(^uint32(0))
	if limited {
		lim = int(ToUint32(limit))
	}
	out := []Value{}
	if lim == 0 {
		return NewArrayValue(out)
	}
	if len(str) == 0 {
		if re.re.FindStringIndex(str) != nil {
			return NewArrayValue(out)
		}
		return NewArrayValue([]Value{StringValue(s)})
	}
	p, q := 0, 0
	for q < len(str) {
		loc := re.re.FindStringSubmatchIndex(str[q:])
		if loc == nil || loc[0] != 0 {
			q += runeSize(str, q)
			continue
		}
		e := q + loc[1]
		if e == p {
			q += runeSize(str, q)
			continue
		}
		out = append(out, StringValue(FromGoString(str[p:q])))
		if len(out) >= lim {
			return NewArrayValue(out[:lim])
		}
		for i := 1; i < len(loc)/2; i++ {
			lo, hi := loc[2*i], loc[2*i+1]
			if lo < 0 {
				out = append(out, Undefined)
			} else {
				out = append(out, StringValue(FromGoString(str[q+lo:q+hi])))
			}
			if len(out) >= lim {
				return NewArrayValue(out[:lim])
			}
		}
		p = e
		q = p
	}
	out = append(out, StringValue(FromGoString(str[p:])))
	if len(out) > lim {
		out = out[:lim]
	}
	return NewArrayValue(out)
}

// expandReplacement expands one match's replacement template into its result text,
// applying the ECMAScript substitution patterns (22 §22.1.3.19.1 GetSubstitution):
// $$ is a literal dollar, $& the whole match, $` and $' the text before and after
// the match, and $n or $nn a capture group's text (or the empty string for a group
// that did not participate). A $ that begins none of these, a $ before a group
// number past the last group, is copied literally the way the specification leaves
// it. Offsets are byte offsets into the UTF-8 subject, which slice the same text the
// UTF-16 positions name for the code points RE2 hosts.
func expandReplacement(str string, m []int, tmpl string) string {
	nGroups := len(m)/2 - 1
	var b strings.Builder
	for i := 0; i < len(tmpl); i++ {
		if tmpl[i] != '$' || i+1 >= len(tmpl) {
			b.WriteByte(tmpl[i])
			continue
		}
		switch c := tmpl[i+1]; {
		case c == '$':
			b.WriteByte('$')
			i++
		case c == '&':
			b.WriteString(str[m[0]:m[1]])
			i++
		case c == '`':
			b.WriteString(str[:m[0]])
			i++
		case c == '\'':
			b.WriteString(str[m[1]:])
			i++
		case c >= '0' && c <= '9':
			n, adv := groupRef(tmpl, i+1, nGroups)
			if n < 0 {
				b.WriteByte('$')
				continue
			}
			lo, hi := m[2*n], m[2*n+1]
			if lo >= 0 {
				b.WriteString(str[lo:hi])
			}
			i += adv
		default:
			b.WriteByte('$')
		}
	}
	return b.String()
}

// groupRef reads a capture-group reference at position i in the template, where
// tmpl[i] is already known to be a digit. It prefers the two-digit form $nn when
// those two digits name a group in range, else the one-digit form $n, and returns
// the group number and how many digit characters it consumed. It returns -1 when no
// digit run names a group in range, the case GetSubstitution leaves as literal text.
func groupRef(tmpl string, i, nGroups int) (group, advance int) {
	if i+1 < len(tmpl) && tmpl[i+1] >= '0' && tmpl[i+1] <= '9' {
		nn := int(tmpl[i]-'0')*10 + int(tmpl[i+1]-'0')
		if nn >= 1 && nn <= nGroups {
			return nn, 2
		}
	}
	n := int(tmpl[i] - '0')
	if n >= 1 && n <= nGroups {
		return n, 1
	}
	return -1, 0
}

// runeSize reports the byte width of the UTF-8 rune starting at byte offset i in s,
// the one-code-point step split takes past an empty or non-matching position. It is
// the AdvanceStringIndex the specification applies, in the code-point unit RE2 hosts.
func runeSize(s string, i int) int {
	if i >= len(s) {
		return 1
	}
	_, size := utf8.DecodeRuneInString(s[i:])
	if size == 0 {
		return 1
	}
	return size
}
