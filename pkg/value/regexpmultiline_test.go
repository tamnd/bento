package value

import "testing"

// ECMAScript breaks a line at four characters, \n \r U+2028 and U+2029, while RE2
// breaks one only at \n. An anchored multiline pattern therefore searches a subject
// whose other three terminators have been rewritten to \n. These pin that the rewrite
// puts ^ and $ where the language puts them and that nothing else about the match
// moves: the positions the language reports are counted in UTF-16 units, which the
// rewrite preserves, while the byte offsets a captured substring is cut at are not,
// which is what the mapping back exists for.

// TestAMultilineAnchorBreaksAtEveryTerminator is the whole of the gap: the same
// pattern, the same shape of subject, one terminator each.
func TestAMultilineAnchorBreaksAtEveryTerminator(t *testing.T) {
	for _, sep := range []string{"\n", "\r", "\u2028", "\u2029"} {
		re := NewRegExpLiteral("^two$", "m")
		if !re.Test(FromGoString("one" + sep + "two" + sep + "three")) {
			t.Errorf("^two$ did not match a line broken by %q", sep)
		}
	}
}

// TestAMultilineAnchorStillNeedsABreak keeps the rewrite from inventing boundaries. A
// subject with no terminator has one line, so an anchor in the middle of it matches
// nothing.
func TestAMultilineAnchorStillNeedsABreak(t *testing.T) {
	re := NewRegExpLiteral("^two$", "m")
	if re.Test(FromGoString("one two three")) {
		t.Error("^two$ matched inside a subject with no line break")
	}
}

// TestACarriageReturnNewlinePairBreaksOnce pins the \r\n case the specification is
// careful about: the pair is two terminators, so the position between them is both the
// end of one line and the start of the next, and a line's text carries neither.
func TestACarriageReturnNewlinePairBreaksOnce(t *testing.T) {
	re := NewRegExpLiteral("^(bb)$", "m")
	m := re.Exec(FromGoString("a\r\nbb\r\nccc"))
	if m == Null {
		t.Fatal("^(bb)$ did not match across a \\r\\n pair")
	}
	if got := ToString(m.GetIndex(1)).ToGoString(); got != "bb" {
		t.Errorf("captured %q, want bb", got)
	}
}

// TestAMatchPastASeparatorReportsUnitPositions is the offset half. U+2028 is one
// UTF-16 unit and three UTF-8 bytes, so a match after one lands at a different byte
// offset in the searched text than in the subject; .index counts units and must not
// notice, while the captured text is cut from the subject and must.
func TestAMatchPastASeparatorReportsUnitPositions(t *testing.T) {
	re := NewRegExpLiteral(`^(t\w+)$`, "m")
	m := re.Exec(FromGoString("one\u2028two\u2029three"))
	if m == Null {
		t.Fatal("the pattern did not match past a separator")
	}
	if got := ToNumber(m.Get(FromGoString("index"))); got != 4 {
		t.Errorf(".index = %v, want 4", got)
	}
	if got := ToString(m.GetIndex(1)).ToGoString(); got != "two" {
		t.Errorf("captured %q, want two", got)
	}
	if got := ToString(m.GetIndex(0)).ToGoString(); got != "two" {
		t.Errorf("matched %q, want two", got)
	}
}

// TestAReplaceAcrossSeparatorsCutsTheSubject is the same mapping through the other
// path that reads byte offsets, where getting it wrong would splice the result at the
// wrong place rather than merely report a wrong number.
func TestAReplaceAcrossSeparatorsCutsTheSubject(t *testing.T) {
	re := NewRegExpLiteral("^(two)$", "m")
	got := re.ReplaceStr(FromGoString("one\u2028two\u2029three"), FromGoString("[$1]")).ToGoString()
	if want := "one\u2028[two]\u2029three"; got != want {
		t.Errorf("replace gave %q, want %q", got, want)
	}
}

// TestASearchPastASeparatorReportsAUnitOffset covers the third path, whose answer is
// a position rather than text.
func TestASearchPastASeparatorReportsAUnitOffset(t *testing.T) {
	re := NewRegExpLiteral("^two$", "m")
	if got := re.Search(FromGoString("one\u2028two\u2029three")); got != 4 {
		t.Errorf("search gave %v, want 4", got)
	}
}

// TestASubjectWithNoExtraTerminatorIsNotRewritten pins that the ordinary subject takes
// the ordinary path. The rewrite exists for three characters; a subject holding none of
// them is searched as it stands.
func TestASubjectWithNoExtraTerminatorIsNotRewritten(t *testing.T) {
	re := NewRegExpLiteral("^b$", "m")
	ns := re.searchText("a\nb\nc")
	if ns.shifts != nil {
		t.Errorf("a subject with no separator collected shifts: %v", ns.shifts)
	}
	if ns.text != "a\nb\nc" {
		t.Errorf("a subject with no separator was rewritten to %q", ns.text)
	}
}

// TestAnUnanchoredMultilinePatternIsNotRewritten pins the other half of the same
// narrowness. Without a ^ or $ the flag changes nothing a match can see, so a pattern
// that reads a \r directly keeps reading it.
func TestAnUnanchoredMultilinePatternIsNotRewritten(t *testing.T) {
	re := NewRegExpLiteral(`a\rb`, "m")
	if !re.Test(FromGoString("a\rb")) {
		t.Error("an unanchored multiline pattern stopped matching a carriage return")
	}
}

// TestAPatternThatTellsTerminatorsApart pins the guard the rewrite rests on, since the
// rewrite is faithful exactly when the pattern cannot tell one terminator from
// another. A pattern that can still hands back at translation, which is what keeps a
// \r from silently becoming a \n underneath it.
func TestAPatternThatTellsTerminatorsApart(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		apart   bool
	}{
		{"an ordinary pattern", `^Hardware\s*:\s*(.*)$`, false},
		{"a class with no terminator", `^[a-z ]+$`, false},
		{"a negated class", `^[^0-9]+$`, false},
		{"a range holding all four", `^[\x00-\uffff]$`, false},
		{"a literal carriage return", `^a\rb$`, true},
		{"a literal newline", `^a\nb$`, true},
		{"a class holding one", `^[\n]$`, true},
		{"a negated class holding one", `^[^\r]$`, true},
		{"a range splitting them", `^[\x00-\x20]$`, true},
		{"a class keeping RE2's \\s", `^[\s]$`, true},
		{"a control escape", `^a\cM$`, true},
		{"a hex escape", `^a\x0a$`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternTellsTerminatorsApart(tc.pattern); got != tc.apart {
				t.Errorf("patternTellsTerminatorsApart(%q) = %v, want %v", tc.pattern, got, tc.apart)
			}
		})
	}
}
