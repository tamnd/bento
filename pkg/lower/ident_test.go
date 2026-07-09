package lower

import "testing"

// TestMangleIdentBlank pins that the lone underscore, Go's blank identifier,
// mangles to a readable spelling rather than passing through. A JavaScript
// binding or property named _ is an ordinary readable name, so emitting Go's _
// would discard its value and refuse to compile when it is read.
func TestMangleIdentBlank(t *testing.T) {
	got, ok := mangleIdent("_")
	if !ok {
		t.Fatal("mangleIdent(_) reported not ok")
	}
	if got == "_" {
		t.Fatal("mangleIdent(_) passed the blank identifier through; it cannot be read in Go")
	}
	if got != "U5F_" {
		t.Fatalf("mangleIdent(_) = %q, want U5F_", got)
	}
	// A double underscore is a legal, readable Go identifier and must be left
	// alone, so only the single blank is escaped.
	if got, _ := mangleIdent("__"); got != "__" {
		t.Fatalf("mangleIdent(__) = %q, want __ untouched", got)
	}
}

// TestLocalAndFieldBlank pins that both a local and an exported field derived
// from the blank name land on the same readable spelling, so a declaration and
// every reference agree.
func TestLocalAndFieldBlank(t *testing.T) {
	local, ok := localName("_")
	if !ok || local != "U5F_" {
		t.Fatalf("localName(_) = %q, %v; want U5F_, true", local, ok)
	}
	field, ok := exportedField("_")
	if !ok || field != "U5F_" {
		t.Fatalf("exportedField(_) = %q, %v; want U5F_, true", field, ok)
	}
}
