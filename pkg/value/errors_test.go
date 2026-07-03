package value

import "testing"

// TestErrorFormatsLikeJavaScript checks the string an error renders to matches
// Error.prototype.toString: name and message joined by ": ", or the bare name when
// the message is empty.
func TestErrorFormatsLikeJavaScript(t *testing.T) {
	if got := NewTypeError(FromGoString("not a function")).Error(); got != "TypeError: not a function" {
		t.Errorf("error text = %q, want %q", got, "TypeError: not a function")
	}
	if got := NewError(FromGoString("")).Error(); got != "Error" {
		t.Errorf("empty-message error text = %q, want %q", got, "Error")
	}
}

// TestConstructorsCarryTheirName checks each constructor stamps the JavaScript
// error name its new expression names, since a catch and the reporter tell errors
// apart by name.
func TestConstructorsCarryTheirName(t *testing.T) {
	cases := []struct {
		err  *Error
		name string
	}{
		{NewError(FromGoString("x")), "Error"},
		{NewTypeError(FromGoString("x")), "TypeError"},
		{NewRangeError(FromGoString("x")), "RangeError"},
	}
	for _, tc := range cases {
		if tc.err.ErrorName() != tc.name {
			t.Errorf("ErrorName = %q, want %q", tc.err.ErrorName(), tc.name)
		}
		if tc.err.ErrorMessage() != "x" {
			t.Errorf("ErrorMessage = %q, want %q", tc.err.ErrorMessage(), "x")
		}
	}
}

// TestErrorIsThrown proves a constructed error satisfies the Thrown marker, so the
// top-level reporter and a catch recognize it as a deliberate throw rather than a
// Go runtime panic.
func TestErrorIsThrown(t *testing.T) {
	var _ Thrown = NewError(FromGoString("x"))
	var _ error = NewError(FromGoString("x"))
}

// TestThrowPanicsWithTheError proves Throw raises the error as the panic payload,
// unchanged, so a recover downstream gets the same *Error to bind into a catch.
func TestThrowPanicsWithTheError(t *testing.T) {
	want := NewRangeError(FromGoString("out of range"))
	defer func() {
		got := recover()
		if got != want {
			t.Fatalf("recovered %v, want the thrown error %v", got, want)
		}
	}()
	Throw(want)
	t.Fatal("Throw returned instead of panicking")
}

// TestReportUncaughtRepanicsNonThrown proves the reporter leaves a Go runtime
// panic alone: a payload that is not a thrown value is re-panicked so its original
// crash and stack survive rather than being dressed up as a caught error. The
// deliberate-throw path exits the process, so it is covered end to end by the
// lowering test that runs a throwing program rather than here.
func TestReportUncaughtRepanicsNonThrown(t *testing.T) {
	defer func() {
		if got := recover(); got != "a runtime bug" {
			t.Fatalf("re-panicked %v, want the original payload", got)
		}
	}()
	func() {
		defer ReportUncaught()
		panic("a runtime bug")
	}()
	t.Fatal("ReportUncaught swallowed a non-thrown panic")
}
