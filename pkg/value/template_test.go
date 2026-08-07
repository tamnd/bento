package value

import "testing"

// strs builds a template strings object from parallel cooked and raw parts, the way a
// compiled call site does, so each case below reads like the template it stands for.
func strs(cooked, raw []string) Value {
	cv := make([]Value, len(cooked))
	for i, s := range cooked {
		cv[i] = StringValue(FromGoString(s))
	}
	rv := make([]Value, len(raw))
	for i, s := range raw {
		rv[i] = StringValue(FromGoString(s))
	}
	return TemplateObject(cv, rv)
}

// TestTemplateObjectShape pins what a tag sees: the cooked parts as the array's own
// elements, the raw parts under raw, and a length that counts the cooked parts.
func TestTemplateObjectShape(t *testing.T) {
	o := strs([]string{"a\n", "b"}, []string{`a\n`, "b"})
	if got := ToNumber(o.Get(FromGoString("length"))); got != 2 {
		t.Errorf("length = %v, want 2", got)
	}
	if got := ToString(o.GetIndex(0)).ToGoString(); got != "a\n" {
		t.Errorf("cooked[0] = %q, want %q", got, "a\n")
	}
	raw := o.Get(FromGoString("raw"))
	if got := ToString(raw.GetIndex(0)).ToGoString(); got != `a\n` {
		t.Errorf("raw[0] = %q, want %q", got, `a\n`)
	}
}

// TestTemplateObjectIsFrozen pins that a tag cannot change what the next call sees. The
// language freezes both arrays and the object, so a write drops rather than sticks.
func TestTemplateObjectIsFrozen(t *testing.T) {
	o := strs([]string{"a"}, []string{"a"})
	if !o.IsFrozen() {
		t.Error("the strings object is not frozen")
	}
	if !o.Get(FromGoString("raw")).IsFrozen() {
		t.Error("the raw array is not frozen")
	}
	o.Set(FromGoString("extra"), Number(1))
	if o.Get(FromGoString("extra")).Kind() != KindUndefined {
		t.Error("a write to the frozen strings object stuck")
	}
}

// TestStringRaw pins the built-in's walk: one raw segment, then one substitution, until
// the last segment, which is not followed by one.
func TestStringRaw(t *testing.T) {
	cases := []struct {
		name string
		raw  []string
		subs []Value
		want string
	}{
		{"no substitution", []string{`a\nb`}, nil, `a\nb`},
		{"one substitution", []string{"p", "q"}, []Value{Number(1)}, "p1q"},
		{"two substitutions", []string{"p", "q", "r"}, []Value{Number(1), StringValue(FromGoString("x"))}, "p1qxr"},
		{"coerces a substitution", []string{"<", ">"}, []Value{Undefined}, "<undefined>"},
		{"extra substitutions are ignored", []string{"a", "b"}, []Value{Number(1), Number(2)}, "a1b"},
		{"missing substitutions run the segments together", []string{"a", "b", "c"}, []Value{Number(1)}, "a1bc"},
		{"no parts", nil, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := strs(c.raw, c.raw)
			if got := StringRaw(o, c.subs...).ToGoString(); got != c.want {
				t.Errorf("StringRaw = %q, want %q", got, c.want)
			}
		})
	}
}

// TestStringRawReadsRawOffAnyObject pins that the built-in reads a raw property rather
// than a template object specifically, which is what lets a tag forward its own parts
// through String.raw({ raw: parts }, ...).
func TestStringRawReadsRawOffAnyObject(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("raw"), NewArrayValue([]Value{
		StringValue(FromGoString("u")), StringValue(FromGoString("v")),
	}))
	if got := StringRaw(o, Number(7)).ToGoString(); got != "u7v" {
		t.Errorf("StringRaw on a hand-built object = %q, want %q", got, "u7v")
	}
}

// TestStringRawArgs pins the array-taking form, which is what a spread of a rest
// parameter lowers to: it reads the substitutions off the array and answers what the
// spelled-out call answers.
func TestStringRawArgs(t *testing.T) {
	o := strs([]string{"p", "q", "r"}, []string{"p", "q", "r"})
	subs := NewArrayValue([]Value{Number(1), Number(2)})
	if got := StringRawArgs(o, subs).ToGoString(); got != "p1q2r" {
		t.Errorf("StringRawArgs = %q, want %q", got, "p1q2r")
	}
	if got := StringRawArgs(o, NewArrayValue(nil)).ToGoString(); got != "pqr" {
		t.Errorf("StringRawArgs with no substitutions = %q, want %q", got, "pqr")
	}
}
