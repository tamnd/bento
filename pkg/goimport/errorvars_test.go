package goimport

import "testing"

// TestErrorVarsFindsSentinel proves the classifier reports a package-level error
// variable: io.EOF is the canonical sentinel a caught error's is() compares
// against, so it must come back in the set, while a non-error export from the same
// package (io.Discard, an io.Writer) must not.
func TestErrorVarsFindsSentinel(t *testing.T) {
	vars, err := ErrorVars("io")
	if err != nil {
		t.Fatalf("ErrorVars(io) errored: %v", err)
	}
	if !vars["EOF"] {
		t.Error("io.EOF not reported as an error variable, want it in the set")
	}
	if vars["Discard"] {
		t.Error("io.Discard reported as an error variable, want it excluded")
	}
}

// TestErrorVarsExcludesConstsAndFuncs proves only variables qualify: strconv
// exports ErrSyntax and ErrRange as error variables, so both must be present, and
// its Atoi function and IntSize constant must not, since a function or a constant
// is never an error sentinel even when a value of it could be an error.
func TestErrorVarsExcludesConstsAndFuncs(t *testing.T) {
	vars, err := ErrorVars("strconv")
	if err != nil {
		t.Fatalf("ErrorVars(strconv) errored: %v", err)
	}
	if !vars["ErrSyntax"] || !vars["ErrRange"] {
		t.Errorf("strconv error sentinels missing: %v", vars)
	}
	if vars["Atoi"] {
		t.Error("strconv.Atoi reported as an error variable, want the function excluded")
	}
	if vars["IntSize"] {
		t.Error("strconv.IntSize reported as an error variable, want the constant excluded")
	}
}

// TestErrorVarsEmptyForPureData proves a package with no exported error variable
// reports an empty set rather than an error, so err.is against a binding from such
// a package simply hands back. math exports numbers and functions and no error.
func TestErrorVarsEmptyForPureData(t *testing.T) {
	vars, err := ErrorVars("math")
	if err != nil {
		t.Fatalf("ErrorVars(math) errored: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("math reported error variables %v, want none", vars)
	}
}
