package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// renderProgramHandBack compiles a module and asserts the assembler hands it
// back as NotYetLowerable, the boundary a class construct outside the covered
// subset must keep, and returns the reason so a case can pin which rule fired.
func renderProgramHandBack(t *testing.T, src string) string {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	return nyl.Reason
}

// TestClassEmitsStruct pins the type half of the class lowering: the class
// becomes a named struct with one exported field per declared property, each
// tagged with the source property name, and a local holding an instance is
// typed as a pointer to it.
func TestClassEmitsStruct(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
}
const p: Point = new Point(3, 4);
console.log(p.x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "type Point struct {") {
		t.Errorf("class did not lower to a named struct:\n%s", source)
	}
	if !strings.Contains(source, "X float64 `json:\"x\"`") || !strings.Contains(source, "Y float64 `json:\"y\"`") {
		t.Errorf("class fields did not lower to tagged exported fields:\n%s", source)
	}
	if !strings.Contains(source, "p := NewPoint(3, 4)") {
		t.Errorf("instance local did not initialize through the constructor:\n%s", source)
	}
}

// TestClassCtorFoldsComposite pins the constructor fold: a constructor whose
// whole effect is storing pure values into fields prints as the one-line
// composite literal a person writes, not an allocate-assign-return sequence.
func TestClassCtorFoldsComposite(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
}
console.log(new Point(3, 4).x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return &Point{X: x, Y: y}") {
		t.Errorf("pure constructor did not fold to a composite literal:\n%s", source)
	}
}

// TestClassDefaultCtorFoldsInits pins the declared-initializer path: a class
// with field initializers and no constructor gets a zero-argument NewX that
// folds the initializers into the composite literal.
func TestClassDefaultCtorFoldsInits(t *testing.T) {
	const src = `class Counter {
  count: number = 0;
  label: string = "hits";
}
const c: Counter = new Counter();
console.log(c.count);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func NewCounter() *Counter {") {
		t.Errorf("class with no constructor did not get a zero-argument NewCounter:\n%s", source)
	}
	if !strings.Contains(source, "return &Counter{Count: 0, Label:") {
		t.Errorf("field initializers did not fold into the composite literal:\n%s", source)
	}
}

// TestClassCtorGeneralForm pins the unfolded constructor: a body statement that
// is not a pure field store keeps the allocate-assign-return sequence, with the
// lowered body running between the field initializers and the return.
func TestClassCtorGeneralForm(t *testing.T) {
	const src = `class Logger {
  count: number = 0;
  constructor() {
    console.log("born");
  }
}
const l: Logger = new Logger();
console.log(l.count);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "l := &Logger{}") {
		t.Errorf("impure constructor did not keep the general receiver form:\n%s", source)
	}
	if !strings.Contains(source, "l.Count = 0") {
		t.Errorf("field initializer did not assign in the general form:\n%s", source)
	}
	if !strings.Contains(source, "return l") {
		t.Errorf("general constructor did not return the receiver:\n%s", source)
	}
}

// TestClassMethodEmitsReceiver pins the method half: a method becomes a
// pointer-receiver Go method, this reads as the receiver, and a call on an
// instance dispatches to it directly.
func TestClassMethodEmitsReceiver(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  norm2(): number {
    return this.x * this.x + this.y * this.y;
  }
}
const p: Point = new Point(3, 4);
console.log(p.norm2());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (p *Point) Norm2() float64 {") {
		t.Errorf("method did not lower to a pointer-receiver Go method:\n%s", source)
	}
	if !strings.Contains(source, "return p.X*p.X + p.Y*p.Y") {
		t.Errorf("this did not lower to the receiver in the method body:\n%s", source)
	}
	if !strings.Contains(source, "p.Norm2()") {
		t.Errorf("instance method call did not dispatch to the Go method:\n%s", source)
	}
}

// TestClassReceiverAvoidsCollision pins the receiver naming rule: when the
// short receiver name is already an identifier the class body speaks (a
// parameter here), the receiver falls back rather than shadow it.
func TestClassReceiverAvoidsCollision(t *testing.T) {
	const src = `class Circle {
  r: number;
  constructor(c: number) {
    this.r = c;
  }
  area(): number {
    return this.r * this.r * Math.PI;
  }
}
console.log(new Circle(2).area());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (circle *Circle) Area() float64 {") {
		t.Errorf("receiver did not fall back past the colliding short name:\n%s", source)
	}
}

// TestClassFieldStores pins the store shapes on a field: a compound store
// prints as the compound operator, a step of one prints as the increment, and
// both work through this inside a method and through an instance local outside.
func TestClassFieldStores(t *testing.T) {
	const src = `class Counter {
  count: number = 0;
  bump(): void {
    this.count += 1;
  }
  add(n: number): void {
    this.count += n;
  }
}
const c: Counter = new Counter();
c.bump();
c.count += 3;
c.count++;
c.count = c.count + 1;
console.log(c.count);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"c.Count++",    // this.count += 1 inside bump, and the two step-of-one stores outside
		"c.Count += n", // the compound store keeps the compound operator
		"c.Count += 3", // a compound store through the instance local
	} {
		if !strings.Contains(source, want) {
			t.Errorf("field store did not print %q:\n%s", want, source)
		}
	}
}

// TestClassMutatingMethodRuns pins that mutation through a method sticks: the
// receiver is a pointer, so a scale method writes the caller's instance.
func TestClassMutatingMethodRuns(t *testing.T) {
	const src = `class Point {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  scale(f: number): void {
    this.x *= f;
  }
}
const p: Point = new Point(3);
p.scale(2);
console.log(p.x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "p.X *= f") {
		t.Errorf("compound this-store did not lower to the compound operator:\n%s", source)
	}
}

// TestClassStructRoutesBeforeShapes pins the routing order: a class receiver
// is dispatched as the class it is, before the fingerprint paths that would
// otherwise claim its member names.
func TestClassStructRoutesBeforeShapes(t *testing.T) {
	// A class whose only field is named length must lower as the class, not
	// take the array-or-string .length path, and a class with a size field must
	// not take the Map .size path: the class routes are checked first.
	const src = `class Box {
  length: number = 2;
  size: number = 3;
}
const b: Box = new Box();
console.log(b.length + b.size);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Length") || !strings.Contains(source, "b.Size") {
		t.Errorf("class fields named length and size did not read as struct fields:\n%s", source)
	}
}

// TestClassHandsBack pins the boundary: each class construct outside the core
// slice hands the unit back rather than lowering with the wrong semantics.
func TestClassHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"extends",
			"class A { x: number = 1; }\nclass B extends A { }\nconsole.log(new B().x);\n",
			"heritage",
		},
		{
			"staticAccessor",
			"class A { static get n(): number { return 1; } }\nconsole.log(A.n);\n",
			"static accessor",
		},
		{
			"staticAsyncMethod",
			"class A { static async f(): Promise<void> { } }\nA.f();\n",
			"static async",
		},
		{
			"uninitializedStaticField",
			"class A { static n: number; }\nconsole.log(A.n);\n",
			"without an initializer",
		},
		{
			"staticInitReadsName",
			"const base: number = 5;\nclass A { static n: number = base; }\nconsole.log(A.n);\n",
			"constant expression",
		},
		{
			"thisInStaticMethod",
			"class A { static f(): void { console.log(this); } }\nA.f();\n",
			"outside a lowered class body",
		},
		{
			"compoundThroughSetter",
			"class A { n: number = 1; get x(): number { return this.n; } set x(v: number) { this.n = v; } }\nconst a: A = new A();\na.x += 1;\nconsole.log(a.n);\n",
			"through the .x accessor",
		},
		{
			"staticMethodAsValue",
			"class A { static f(): number { return 1; } }\nconst g: () => number = A.f;\nconsole.log(g());\n",
			"read as a value",
		},
		{
			"methodCollidesWithSetter",
			"class A { n: number = 0; set x(v: number) { this.n = v; } setX(m: number): void { this.n = m; } }\nconst a: A = new A();\na.setX(1);\nconsole.log(a.n);\n",
			"setter's Go name",
		},
		{
			"moduleSpeaksNewName",
			"function NewA(): number { return 7; }\nclass A { n: number = 1; }\nconsole.log(NewA() + new A().n);\n",
			"already speaks NewA",
		},
		{
			"definiteAssignment",
			"class A { x!: number; }\nconst a: A = new A();\nconsole.log(a.x);\n",
			"definite-assignment",
		},
		{
			"parameterProperty",
			"class A { constructor(private x: number) { } }\nconst a: A = new A(1);\nconsole.log(a);\n",
			"parameter property",
		},
		{
			"ctorReturn",
			"class A { x: number = 1; constructor() { return; } }\nconsole.log(new A().x);\n",
			"return inside a constructor",
		},
		{
			"thisInInitializer",
			"class A { x: number = 1; y: number = this.x + 1; }\nconsole.log(new A().y);\n",
			"initializer that reads this",
		},
		{
			"methodAsValue",
			"class A { f(): number { return 1; } }\nconst g: () => number = new A().f;\nconsole.log(g());\n",
			"read as a value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}

// TestClassStaticFieldEmitsPackageVar pins the static field lowering: the
// field becomes a package var named after the class, its initializer runs as
// the var initializer, and reads and stores route to the var, with the
// compound and step-of-one collapses a local's stores get.
func TestClassStaticFieldEmitsPackageVar(t *testing.T) {
	const src = `class A {
  static total: number = 0;
}
A.total = 5;
A.total += 3;
A.total++;
A.total = A.total + 1;
console.log(A.total);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"var aTotal float64 = 0", // the static becomes a package var with the initializer
		"aTotal = 5",             // a plain store writes the var
		"aTotal += 3",            // a compound store keeps the compound operator
		"aTotal++",               // the step of one collapses, on ++ and on the spelled-out form
	} {
		if !strings.Contains(source, want) {
			t.Errorf("static field did not print %q:\n%s", want, source)
		}
	}
}

// TestClassStaticMethodEmitsFunc pins the static method lowering: the method
// becomes a package function named after the class, and a static store inside
// its body routes to the package var.
func TestClassStaticMethodEmitsFunc(t *testing.T) {
	const src = `class A {
  static total: number = 0;
  static bump(): number {
    A.total = A.total + 1;
    return A.total;
  }
}
console.log(A.bump());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func ABump() float64 {") {
		t.Errorf("static method did not lower to a package function:\n%s", source)
	}
	if !strings.Contains(source, "aTotal++") {
		t.Errorf("static store inside the static body did not collapse to the increment:\n%s", source)
	}
	if !strings.Contains(source, "return aTotal") {
		t.Errorf("static read inside the static body did not route to the package var:\n%s", source)
	}
	if !strings.Contains(source, "ABump()") {
		t.Errorf("static call did not dispatch to the package function:\n%s", source)
	}
}

// TestClassStaticNameAvoidsCollision pins the static var naming rule: when the
// module already speaks the short name, the var falls back to the class-cased
// spelling rather than shadow it.
func TestClassStaticNameAvoidsCollision(t *testing.T) {
	const src = `const aTotal: number = 1;
class A {
  static total: number = 2;
}
console.log(aTotal + A.total);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var ATotal float64 = 2") {
		t.Errorf("static var did not fall back past the taken short name:\n%s", source)
	}
	if !strings.Contains(source, "aTotal + ATotal") {
		t.Errorf("the local and the static did not keep their distinct names:\n%s", source)
	}
}

// TestClassStaticRoutesBeforeInstance pins the routing order: the class name's
// type shares the class symbol an instance type walks to, so a static access
// must resolve through the class name first or B.v would read the instance
// field.
func TestClassStaticRoutesBeforeInstance(t *testing.T) {
	const src = `class B {
  v: number = 1;
  static v: number = 2;
}
const b: B = new B();
console.log(b.v + B.v);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var bV float64 = 2") {
		t.Errorf("static v did not become its own package var:\n%s", source)
	}
	if !strings.Contains(source, "b.V + bV") {
		t.Errorf("the instance read and the static read did not split:\n%s", source)
	}
}

// TestClassAccessorsEmitMethods pins the accessor lowering: a getter becomes a
// plain method, a setter its Set-prefixed sibling, a property read becomes the
// getter call, and a plain store becomes the setter call.
func TestClassAccessorsEmitMethods(t *testing.T) {
	const src = `class Temp {
  c: number = 0;
  get f(): number {
    return this.c * 9 / 5 + 32;
  }
  set f(v: number) {
    this.c = (v - 32) * 5 / 9;
  }
}
const t: Temp = new Temp();
t.f = 212;
console.log(t.c);
console.log(t.f);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (t *Temp) F() float64 {") {
		t.Errorf("getter did not lower to a plain method:\n%s", source)
	}
	if !strings.Contains(source, "func (t *Temp) SetF(v float64) {") {
		t.Errorf("setter did not lower to the Set-prefixed method:\n%s", source)
	}
	if !strings.Contains(source, "t.SetF(212)") {
		t.Errorf("property store did not lower to the setter call:\n%s", source)
	}
	if !strings.Contains(source, "t.F()") {
		t.Errorf("property read did not lower to the getter call:\n%s", source)
	}
}

// TestClassCompletionRuns builds and runs a program that exercises the whole
// slice end to end: construction through the folded and general constructors,
// method dispatch, this-stores, instance-local stores, and pointer-receiver
// mutation, printing what node prints.
func TestClassCompletionRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  norm2(): number {
    return this.x * this.x + this.y * this.y;
  }
  scale(f: number): void {
    this.x *= f;
    this.y *= f;
  }
}
class Counter {
  count: number = 0;
  label: string = "hits";
  bump(): void {
    this.count += 1;
  }
  report(): string {
    return this.label + ": " + this.count;
  }
}
const p: Point = new Point(3, 4);
console.log(p.norm2());
p.scale(2);
console.log(p.x);
console.log(p.y);
console.log(p.norm2());
const c: Counter = new Counter();
c.bump();
c.bump();
c.count += 3;
c.count++;
console.log(c.report());
const q: Point = new Point(p.x, 0);
q.x++;
console.log(q.x);
`
	got := runProgramGo(t, src)
	want := "25\n" +
		"6\n" +
		"8\n" +
		"100\n" +
		"hits: 6\n" +
		"7\n"
	if got != want {
		t.Fatalf("class completion program printed %q, want %q", got, want)
	}
}

// TestClassStaticsAccessorsRun builds and runs the statics-and-accessors slice
// end to end: a setter store and getter read round-trip through the instance,
// and a static method mutates the static field the class owns, printing what
// node prints.
func TestClassStaticsAccessorsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class A {
  static total: number = 0;
  n: number = 1;
  static bump(): number {
    A.total = A.total + 1;
    return A.total;
  }
  get twice(): number {
    return this.n * 2;
  }
  set twice(v: number) {
    this.n = v / 2;
  }
}
const a: A = new A();
a.twice = 10;
console.log(a.twice);
console.log(A.bump());
console.log(A.total);
`
	got := runProgramGo(t, src)
	want := "10\n" +
		"1\n" +
		"1\n"
	if got != want {
		t.Fatalf("statics and accessors program printed %q, want %q", got, want)
	}
}
