package lower

import (
	"strings"
	"testing"
)

// TestDivergentAccessorFieldBoxes pins that an interface (or type-literal) member
// whose get and set types diverge lowers to a boxed value.Value struct field: a
// write boxes through the value constructor and a read unboxes down to the static
// slot it flows into. A get-type field could not hold the wider set-type write, so
// without the box the write t.foo = 32 dropped a NumOrStr into a value.BStr field
// and did not build.
func TestDivergentAccessorFieldBoxes(t *testing.T) {
	src := `interface Test2 {
    get foo(): string;
    set foo(s: string | number);
    get bar(): string | number;
    set bar(s: string | number | boolean);
}
{
    const t = {} as Test2;
    t.foo = 32;
    let m: string = t.foo;
    t.bar = 42;
    let n: number = t.bar;
    t.bar = false;
    let o = t.bar;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "Foo value.Value") || !strings.Contains(out, "Bar value.Value") {
		t.Fatalf("divergent accessor fields were not boxed to value.Value:\n%s", out)
	}
	// A write boxes the value rather than coercing to the set type.
	if !strings.Contains(out, "t.Foo = value.Number(32)") {
		t.Fatalf("write to a divergent accessor did not box its value:\n%s", out)
	}
	if !strings.Contains(out, "t.Bar = value.Bool(false)") {
		t.Fatalf("out-of-get-type write did not box its value:\n%s", out)
	}
	// A read unboxes down to its declared slot: string and number targets take the
	// ToString and ToNumber the dynamic path emits.
	if !strings.Contains(out, "value.ToString(t.Foo)") {
		t.Fatalf("string read of a divergent accessor did not unbox:\n%s", out)
	}
	if !strings.Contains(out, "value.ToNumber(t.Bar)") {
		t.Fatalf("number read of a divergent accessor did not unbox:\n%s", out)
	}
}

// TestDivergentAccessorClassKeepsMethods guards that the boxed-field rewrite does
// not touch a class receiver: a class with divergent accessors already lowers each
// to a getter method that returns the get type and a setter method that takes the
// set type, so its reads must stay c.Foo() and not be rerouted through the boxed
// dynamic path the interface member takes.
func TestDivergentAccessorClassKeepsMethods(t *testing.T) {
	src := `class Test1 {
    get foo(): string { return "" }
    set foo(s: string | number) {}
}
{
    const t = new Test1();
    t.foo = 32;
    let m: string = t.foo;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "t.Foo()") {
		t.Fatalf("class getter read was not a method call:\n%s", out)
	}
	if strings.Contains(out, "value.ToString(t.Foo())") {
		t.Fatalf("class getter read was wrongly routed through the dynamic unbox path:\n%s", out)
	}
}

// TestDivergentAccessorRuns builds and runs the full three-block conformance shape,
// a class, an interface, and a type literal each with the same divergent accessors
// written out of their get type, and proves it compiles and completes with no
// output, matching the runnable JavaScript tsc emits for it.
func TestDivergentAccessorRuns(t *testing.T) {
	skipIfShort(t)
	src := `class Test1 {
    get foo(): string { return "" }
    set foo(s: string | number) {}
    get bar(): string | number { return "" }
    set bar(s: string | number | boolean) {}
}
interface Test2 {
    get foo(): string;
    set foo(s: string | number);
    get bar(): string | number;
    set bar(s: string | number | boolean);
}
type Test3 = {
    get foo(): string;
    set foo(s: string | number);
    get bar(): string | number;
    set bar(s: string | number | boolean);
};
{
    const t = new Test1();
    t.foo = 32;
    let m: string = t.foo;
    t.bar = 42;
    let n: number = t.bar;
    t.bar = false;
    let o = t.bar;
}
{
    const t = {} as Test2;
    t.foo = 32;
    let m: string = t.foo;
    t.bar = 42;
    let n: number = t.bar;
    t.bar = false;
    let o = t.bar;
}
{
    const t = {} as Test3;
    t.foo = 32;
    let m: string = t.foo;
    t.bar = 42;
    let n: number = t.bar;
    t.bar = false;
    let o = t.bar;
}
`
	out := renderProgramTolerant(t, src)
	if got := goRunSource(t, out); got != "" {
		t.Fatalf("divergent accessor run mismatch:\n got %q\nwant %q", got, "")
	}
}
