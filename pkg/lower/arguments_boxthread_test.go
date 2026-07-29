package lower

import (
	"strings"
	"testing"
)

// A function expression boxed straight into a dynamic value that reads its arguments
// object threads the real call-site arguments through a hidden trailing parameter the
// boxed wrapper fills, in place of the parameter snapshot that only matches a call
// passing one argument per parameter. Before this slice such a boxing handed the unit
// back at "arguments in a function boxed into a dynamic value needs the call-site
// count". These cover the render shape (the hidden parameter and the wrapper that
// passes the whole argument slice) and the run across the arity boundary (a count over
// and under the parameter count, an index past the parameters).

// TestBoxedArgumentsFuncExprThreads pins the render: the boxed function expression
// takes the hidden arguments parameter and the wrapper passes the real call-site
// slice, rather than handing back.
func TestBoxedArgumentsFuncExprThreads(t *testing.T) {
	const src = `
let f: any = function(a: number): number { return arguments.length; };
console.log(f(1, 2, 3));
`
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "value.Array[value.Value]") {
		t.Errorf("the boxed function expression did not take a hidden arguments store:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[value.Value](__a...)") {
		t.Errorf("the wrapper did not pass the real call-site arguments to the hidden store:\n%s", source)
	}
}

// TestBoxedArgumentsLengthRuns builds and runs a boxed arguments-reading function
// called at three different arities. The threaded store reads the arity actually
// passed at each dynamic call, not the two the parameters declare.
func TestBoxedArgumentsLengthRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let f: any = function(a: number, b: number): number { return arguments.length; };
console.log(f(1, 2, 3));
console.log(f(9));
console.log(f());
`
	got := runProgramGoTolerant(t, src)
	want := "3\n1\n0\n"
	if got != want {
		t.Fatalf("boxed arguments.length run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestBoxedArgumentsIndexRuns builds and runs a boxed function that concatenates its
// arguments by index across the real call arity, including a slot past the last
// parameter. The hidden store carries every argument the dynamic call passed, so the
// index beyond the parameters resolves to the value actually passed.
func TestBoxedArgumentsIndexRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let join: any = function(a: number): string {
  let s = "";
  for (let i = 0; i < arguments.length; i++) {
    s += arguments[i];
  }
  return s;
};
console.log(join(1, 2, 3));
console.log(join(7));
`
	got := runProgramGoTolerant(t, src)
	want := "123\n7\n"
	if got != want {
		t.Fatalf("boxed arguments index run mismatch:\n got %q\nwant %q", got, want)
	}
}
