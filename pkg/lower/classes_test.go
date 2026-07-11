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
			"implements",
			"interface HasX { x: number; }\nclass A implements HasX { x: number = 1; }\nconsole.log(new A().x);\n",
			"heritage (implements)",
		},
		{
			"extendsNonClass",
			"const A = class { x: number = 1; };\nclass B extends A { }\nconsole.log(new B().x);\n",
			"not a class this slice lowers",
		},
		{
			"shadowField",
			"class A { x: number = 1; }\nclass B extends A { x: number = 2; }\nconsole.log(new B().x);\n",
			"only a method overriding a same-named base method",
		},
		{
			"crossCaseShadow",
			"class A { total(): number { return 1; } }\nclass B extends A { Total: number = 2; }\nconsole.log(new B().Total);\n",
			"only a method overriding a same-named base method",
		},
		{
			"midChainOverride",
			"class A { }\nclass B extends A { m(): number { return 1; } }\nclass C extends B { m(): number { return 2; } }\nconsole.log(new C().m());\n",
			"mid-chain virtual method",
		},
		{
			"overrideSignatureDiffers",
			"class A { m(x: number): number { return x; } }\nclass B extends A { m(): number { return 2; } }\nconsole.log(new B().m());\n",
			"differs from A's",
		},
		{
			"accessorOverride",
			"class A { get n(): number { return 1; } }\nclass B extends A { get n(): number { return 2; } }\nconsole.log(new B().n);\n",
			"only a method overriding a same-named base method",
		},
		{
			"stringifyExtendedClass",
			"class A { x: number = 1; }\nclass B extends A { }\nconsole.log(JSON.stringify(new A()));\n",
			"JSON.stringify of a value typed as class A",
		},
		{
			"memberSpellsBaseName",
			"class A { x: number = 1; }\nclass B extends A { a: number = 2; }\nconsole.log(new B().a);\n",
			"embedded base field's name",
		},
		{
			"abstractField",
			"abstract class A { abstract x: number; }\nconsole.log(\"x\");\n",
			"an abstract field",
		},
		{
			"abstractAccessor",
			"abstract class A { abstract get v(): number; }\nconsole.log(\"x\");\n",
			"an abstract accessor",
		},
		{
			"midChainAbstract",
			"abstract class A { m(): number { return 1; } }\nabstract class B extends A { abstract p(): number; }\nconsole.log(\"x\");\n",
			"mid-chain virtual method",
		},
		{
			"overloadSignature",
			"class A { m(x: number): number; m(x: number): number { return x; } }\nconsole.log(new A().m(1));\n",
			"a method overload signature",
		},
		{
			"declBeforeSuper",
			"class A { x: number = 1; }\nclass B extends A { constructor() { const y: number = 2; super(); } }\nconsole.log(new B().x);\n",
			"variable declaration before super()",
		},
		{
			"uninitializedStaticField",
			"class A { static n: number; }\nconsole.log(A.n);\n",
			"without an initializer",
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
			"parameterPropertyDefault",
			"class A { constructor(public x: number = 5) { } }\nconst a: A = new A();\nconsole.log(a.x);\n",
			"default value",
		},
		{
			"ctorReturn",
			"class A { x: number = 1; constructor() { return; } }\nconsole.log(new A().x);\n",
			"return inside a constructor",
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

// TestClassParamPropsFold pins the parameter property lowering: the parameter
// declares a struct field, and a constructor whose whole effect is the
// parameter properties folds to the composite literal.
func TestClassParamPropsFold(t *testing.T) {
	const src = `class Point {
  constructor(public x: number, public y: number) {}
}
const p: Point = new Point(3, 4);
console.log(p.x + p.y);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "X float64 `json:\"x\"`") || !strings.Contains(source, "Y float64 `json:\"y\"`") {
		t.Errorf("parameter properties did not declare tagged struct fields:\n%s", source)
	}
	if !strings.Contains(source, "return &Point{X: x, Y: y}") {
		t.Errorf("parameter properties did not fold to a composite literal:\n%s", source)
	}
	if !strings.Contains(source, "p.X + p.Y") {
		t.Errorf("parameter property fields did not read as struct fields:\n%s", source)
	}
}

// TestClassParamPropsOrder pins the assignment order: TypeScript assigns
// parameter properties before the declared field initializers run, so in the
// general constructor form the parameter store comes first.
func TestClassParamPropsOrder(t *testing.T) {
	const src = `class A {
  n: number = 1;
  constructor(public m: number) {
    console.log("born");
  }
}
const a: A = new A(2);
console.log(a.m + a.n);
`
	source := renderProgram(t, src)
	iM := strings.Index(source, "a.M = m")
	iN := strings.Index(source, "a.N = 1")
	if iM < 0 || iN < 0 {
		t.Fatalf("general constructor did not assign both the parameter property and the initializer:\n%s", source)
	}
	if iM > iN {
		t.Errorf("parameter property assigned after the declared initializer:\n%s", source)
	}
}

// TestClassParamPropModifiers pins that private and readonly are checker-level
// facts: the field lowers the same exported way, mixed modifiers are accepted,
// and access control stays the checker's job.
func TestClassParamPropModifiers(t *testing.T) {
	const src = `class Box {
  constructor(private readonly v: string) {}
  val(): string {
    return this.v;
  }
}
const b: Box = new Box("hi");
console.log(b.val());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return &Box{V: v}") {
		t.Errorf("private readonly parameter property did not fold:\n%s", source)
	}
	if !strings.Contains(source, "return b.V") {
		t.Errorf("this-read of the parameter property did not lower to the field:\n%s", source)
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

// TestClassParamPropsRun builds and runs the parameter property slice end to
// end: folded construction from parameters, a private readonly field read
// through a method, and a parameter property mixing with a declared field,
// printing what node prints.
func TestClassParamPropsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Point {
  constructor(public x: number, public y: number) {}
  norm2(): number {
    return this.x * this.x + this.y * this.y;
  }
}
class Tag {
  count: number = 0;
  constructor(private readonly name: string) {}
  bump(): string {
    this.count += 1;
    return this.name + ":" + this.count;
  }
}
const p: Point = new Point(3, 4);
console.log(p.norm2());
p.x += 1;
console.log(p.x);
const t: Tag = new Tag("hit");
t.bump();
console.log(t.bump());
`
	got := runProgramGo(t, src)
	want := "25\n" +
		"4\n" +
		"hit:2\n"
	if got != want {
		t.Fatalf("parameter property program printed %q, want %q", got, want)
	}
}

// TestClassExtendsEmbedsAndFolds pins the inheritance lowering: the derived
// struct embeds the base as its first field, a constructor whose statements
// past super() are pure stores folds to the composite literal with the base
// element first, and an inherited method call and field read reach the base
// through Go promotion, with no wrapper emitted.
func TestClassExtendsEmbedsAndFolds(t *testing.T) {
	const src = `class Animal {
  legs: number;
  constructor(legs: number) {
    this.legs = legs;
  }
  count(): number {
    return this.legs;
  }
}
class Dog extends Animal {
  tricks: number;
  constructor(legs: number, tricks: number) {
    super(legs);
    this.tricks = tricks;
  }
}
const d: Dog = new Dog(4, 2);
console.log(d.count());
console.log(d.legs);
console.log(d.tricks);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type Dog struct {\n\tAnimal\n",                         // the embedded base, first
		"return &Dog{Animal: *NewAnimal(legs), Tricks: tricks}", // the fold, base element first
		"d.Count()", // inherited method through promotion
		"d.Legs",    // inherited field through promotion
	} {
		if !strings.Contains(source, want) {
			t.Errorf("inheritance did not print %q:\n%s", want, source)
		}
	}
}

// TestClassExtendsGeneralForm pins the constructor order when the body does
// not fold: the base assignment from super() comes first, then the derived
// field initializers, then the rest of the body with the super statement
// stripped, matching the order JavaScript runs them in.
func TestClassExtendsGeneralForm(t *testing.T) {
	const src = `class A {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
}
class B extends A {
  y: number = 1;
  constructor(x: number) {
    super(x + 1);
    console.log(this.y);
  }
}
console.log(new B(3).x);
`
	source := renderProgram(t, src)
	base := strings.Index(source, "b.A = *NewA(x + 1)")
	init := strings.Index(source, "b.Y = 1")
	if base < 0 || init < 0 || base > init {
		t.Errorf("general form did not order the base assignment before the field initializer:\n%s", source)
	}
	if !strings.Contains(source, "NewB(3).X") {
		t.Errorf("inherited field read on a new expression did not promote:\n%s", source)
	}
}

// TestClassExtendsSynthesizedCtor pins the derived class with no constructor:
// the synthesized NewB takes the base constructor's parameters and passes them
// straight through.
func TestClassExtendsSynthesizedCtor(t *testing.T) {
	const src = `class A {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
}
class B extends A {
}
const b: B = new B(7);
console.log(b.x);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func NewB(x float64) *B",
		"return &B{A: *NewA(x)}",
		"b.X",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("synthesized constructor did not print %q:\n%s", want, source)
		}
	}
}

// TestClassSuperMemberAccess pins super.m() inside a derived method: super
// lowers to the embedded base selector, so the call goes through the base
// value explicitly, while this.n still reads the promoted base field. (A base
// field through super, super.n, is a checker error, so only methods and
// accessors ever reach the super lowering.)
func TestClassSuperMemberAccess(t *testing.T) {
	const src = `class A {
  n: number = 1;
  m(): number {
    return this.n;
  }
}
class B extends A {
  call(): number {
    return super.m() + this.n;
  }
}
console.log(new B().call());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.A.M()") || !strings.Contains(source, "b.N") {
		t.Errorf("super method call did not lower to the embedded base selector:\n%s", source)
	}
}

// TestClassPromotedFieldStore pins stores into an inherited field: this.count
// inside a derived method and b.count on a derived instance both write the
// promoted base field, keeping the compound and step collapses.
func TestClassPromotedFieldStore(t *testing.T) {
	const src = `class A {
  count: number = 0;
}
class B extends A {
  bump(): void {
    this.count += 2;
  }
}
const b: B = new B();
b.bump();
b.count++;
console.log(b.count);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Count += 2") || !strings.Contains(source, "b.Count++") {
		t.Errorf("promoted field stores did not collapse:\n%s", source)
	}
}

// TestClassExtendsRun builds and runs the inheritance slice end to end: a
// two-level chain with a folded derived constructor, an inherited method and
// field served by promotion, and a grandchild with a synthesized constructor,
// printing what node prints.
func TestClassExtendsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Animal {
  legs: number;
  constructor(legs: number) {
    this.legs = legs;
  }
  count(): number {
    return this.legs;
  }
}
class Dog extends Animal {
  tricks: number = 0;
  constructor() {
    super(4);
  }
  learn(): void {
    this.tricks++;
  }
}
class Puppy extends Dog {
}
const d: Dog = new Dog();
d.learn();
d.learn();
console.log(d.count());
console.log(d.tricks);
console.log(d.legs);
const p: Puppy = new Puppy();
p.learn();
console.log(p.count() + p.tricks);
`
	got := runProgramGo(t, src)
	want := "4\n" +
		"2\n" +
		"4\n" +
		"5\n"
	if got != want {
		t.Fatalf("inheritance program printed %q, want %q", got, want)
	}
}

// TestClassVirtualEmitsVTable pins the virtual dispatch lowering on the
// canonical override: the root grows a vtable struct and an unexported vtable
// pointer, the overridden method's body moves to its Impl name while the
// original name becomes the one-line dispatching entry, the root and the
// override each fill a vtable var (the override through the unsafe downcast
// wrapper), construction splits so the vtable is pinned before init runs, and
// a derived instance bound to a base-typed local upcasts to its embedded base.
func TestClassVirtualEmitsVTable(t *testing.T) {
	const src = `class Animal {
  name: string;
  constructor(name: string) {
    this.name = name;
  }
  speak(): string {
    return this.name + " makes a sound";
  }
}
class Dog extends Animal {
  speak(): string {
    return this.name + " barks";
  }
}
const pet: Animal = new Dog("Rex");
console.log(pet.speak());
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type animalVTable struct {",                                      // the root's vtable struct
		"speak func(a *Animal) value.BStr",                                // one slot per overridden method
		"vtable *animalVTable",                                            // the root's unexported pointer field
		"var animalBaseVTable = animalVTable{speak: (*Animal).speakImpl}", // the root's slots are method expressions
		"var dogVTable = animalVTable{",                                   // the override's own var
		"(*Dog)(unsafe.Pointer(a)).speakImpl()",                           // the downcast wrapper behind the shared start address
		"func (a *Animal) Speak() value.BStr {",                           // the entry keeps the exported name
		"return a.vtable.speak(a)",                                        // and only dispatches
		"func (a *Animal) speakImpl() value.BStr {",                       // the root body under its Impl name
		"func (d *Dog) speakImpl() value.BStr {",                          // the override body likewise
		"a.vtable = &animalBaseVTable",                                    // each constructor pins its class's vtable
		"d.vtable = &dogVTable",
		"func initAnimal(a *Animal, name value.BStr) {",      // the extended root splits out init
		"initAnimal(&d.Animal, name)",                        // which the derived constructor runs on the embedded base
		"pet := &NewDog(value.FromGoString(\"Rex\")).Animal", // the upcast is the embedded base's address
		"pet.Speak()", // and the call keeps its spelling
	} {
		if !strings.Contains(source, want) {
			t.Errorf("virtual dispatch did not print %q:\n%s", want, source)
		}
	}
	// The vtable must be pinned before init runs, so a virtual call inside the
	// base constructor already sees the derived override, the JavaScript order.
	pin := strings.Index(source, "d.vtable = &dogVTable")
	init := strings.Index(source, "initAnimal(&d.Animal, name)")
	if pin < 0 || init < 0 || pin > init {
		t.Errorf("constructor did not pin the vtable before running init:\n%s", source)
	}
}

// TestClassVirtualSuperCallsImpl pins super.m() on a virtual method: the entry
// would re-dispatch through the instance's own vtable and recurse into the
// caller, so the super call goes to the base's Impl directly through the
// embedded base selector.
func TestClassVirtualSuperCallsImpl(t *testing.T) {
	const src = `class Base {
  greet(): string {
    return "base";
  }
}
class Loud extends Base {
  greet(): string {
    return super.greet() + "!";
  }
}
const l: Loud = new Loud();
console.log(l.greet());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "l.Base.greetImpl()") {
		t.Errorf("super on a virtual method did not call the base Impl:\n%s", source)
	}
	if strings.Contains(source, "initBase") || strings.Contains(source, "initLoud") {
		t.Errorf("a chain with nothing to initialize split out init functions:\n%s", source)
	}
}

// TestClassVirtualRuns builds and runs virtual dispatch end to end: a
// base-typed parameter dispatches to each override, a base method calling a
// virtual sibling picks the override, a grandchild without its own overrides
// shares its parent's vtable, and a virtual call inside the base constructor
// already sees the derived override, printing exactly what node prints.
func TestClassVirtualRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Shape {
  kind: string;
  constructor(kind: string) {
    this.kind = kind;
  }
  area(): number {
    return 0;
  }
  describe(): string {
    return this.kind + ":" + this.area();
  }
}
class Square extends Shape {
  side: number;
  constructor(side: number) {
    super("square");
    this.side = side;
  }
  area(): number {
    return this.side * this.side;
  }
}
class Grid extends Square {
}
function show(s: Shape): string {
  return s.describe();
}
console.log(show(new Shape("dot")));
console.log(show(new Square(3)));
console.log(show(new Grid(4)));
const s: Shape = new Square(5);
console.log(s.area());
`
	got := runProgramGo(t, src)
	want := "dot:0\n" +
		"square:9\n" +
		"square:16\n" +
		"25\n"
	if got != want {
		t.Fatalf("virtual dispatch program printed %q, want %q", got, want)
	}
}

// TestClassVirtualCtorDispatchRuns builds and runs the construction-order
// half: JavaScript dispatches to the derived override from the first line of
// the base constructor (the instance is its final class immediately, unlike
// C++), which is exactly what pinning the vtable before init preserves.
func TestClassVirtualCtorDispatchRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Animal {
  name: string;
  constructor(name: string) {
    this.name = name;
    console.log(this.tag());
  }
  tag(): string {
    return "animal " + this.name;
  }
}
class Dog extends Animal {
  tag(): string {
    return "dog " + this.name;
  }
}
const a: Animal = new Animal("generic");
const d: Dog = new Dog("rex");
console.log(a.tag());
console.log(d.tag());
`
	got := runProgramGo(t, src)
	want := "animal generic\n" +
		"dog rex\n" +
		"animal generic\n" +
		"dog rex\n"
	if got != want {
		t.Fatalf("constructor dispatch program printed %q, want %q", got, want)
	}
}

// TestClassVirtualSuperChainRuns builds and runs a three-deep override chain:
// each super.greet() reaches the next ancestor's Impl through promotion, and
// JSON.stringify of a leaf instance flattens the embedded base and skips the
// vtable pointer, printing exactly what node prints.
func TestClassVirtualSuperChainRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Base {
  greet(): string {
    return "base";
  }
}
class Loud extends Base {
  greet(): string {
    return super.greet() + "!";
  }
}
class Louder extends Loud {
  greet(): string {
    return super.greet() + "!";
  }
}
console.log(new Loud().greet());
console.log(new Louder().greet());
class Animal {
  name: string;
  constructor(name: string) {
    this.name = name;
  }
  speak(): string {
    return "...";
  }
}
class Dog extends Animal {
  tricks: number = 0;
  speak(): string {
    return "woof";
  }
}
const d: Dog = new Dog("rex");
console.log(d.speak());
console.log(JSON.stringify(d));
`
	got := runProgramGo(t, src)
	want := "base!\n" +
		"base!!\n" +
		"woof\n" +
		"{\"name\":\"rex\",\"tricks\":0}\n"
	if got != want {
		t.Fatalf("super chain program printed %q, want %q", got, want)
	}
}

// TestClassAbstractEmits pins the abstract half of the vtable lowering: the
// abstract root emits its struct, vtable, and virtual entry but no NewX, the
// abstract method has no Impl body and its base slot panics, and a concrete
// subclass constructs through NewX plus the root's init on the embedded base.
func TestClassAbstractEmits(t *testing.T) {
	const src = `abstract class Shape {
  name: string;
  constructor(name: string) {
    this.name = name;
  }
  abstract area(): number;
  describe(): string {
    return this.name + ":" + this.area();
  }
}
class Square extends Shape {
  side: number;
  constructor(side: number) {
    super("square");
    this.side = side;
  }
  area(): number {
    return this.side * this.side;
  }
}
console.log(new Square(3).describe());
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type shapeVTable struct {",                           // the abstract root still owns the vtable
		"area func(s *Shape) float64",                         // the abstract method claims its slot
		"panic(\"Shape.area is abstract\")",                   // the root's slot panics, unreachable in well-typed code
		"func (s *Shape) Area() float64 {",                    // the entry keeps the exported name
		"return s.vtable.area(s)",                             // and only dispatches
		"func initShape(s *Shape, name value.BStr) {",         // construction is the init alone
		"initShape(&s.Shape, value.FromGoString(\"square\"))", // which the subclass runs on the embedded base
		"func NewSquare(side float64) *Square {",              // the concrete subclass keeps its constructor
		".vtable = &squareVTable",                             // pinning its own vtable
		"(*Square)(unsafe.Pointer(s)).areaImpl()",             // whose slot downcasts to the override
	} {
		if !strings.Contains(source, want) {
			t.Errorf("abstract class did not print %q:\n%s", want, source)
		}
	}
	for _, reject := range []string{
		"func NewShape",            // the checker rejects new on an abstract class, so no constructor exists
		"func (s *Shape) areaImpl", // an abstract method has no body to emit
	} {
		if strings.Contains(source, reject) {
			t.Errorf("abstract class printed %q, which must not exist:\n%s", reject, source)
		}
	}
}

// TestClassAbstractNoVTableEmits pins the split an abstract base forces even
// without virtual dispatch: no vtable machinery appears, but the base emits
// init instead of NewX and the subclass constructor runs it in place.
func TestClassAbstractNoVTableEmits(t *testing.T) {
	const src = `abstract class Base {
  label: string;
  constructor(label: string) {
    this.label = label;
  }
  show(): string {
    return this.label;
  }
}
class Kid extends Base {
  constructor() {
    super("kid");
  }
}
console.log(new Kid().show());
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func initBase(b *Base, label value.BStr) {",
		"initBase(&k.Base, value.FromGoString(\"kid\"))",
		"func NewKid() *Kid {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("abstract base did not print %q:\n%s", want, source)
		}
	}
	for _, reject := range []string{
		"func NewBase", // no constructor for the abstract base
		"VTable",       // nothing here dispatches virtually
		"vtable",
	} {
		if strings.Contains(source, reject) {
			t.Errorf("abstract base printed %q, which must not exist:\n%s", reject, source)
		}
	}
}

// TestClassAbstractRuns builds and runs abstract dispatch end to end: two
// concrete subclasses fill the abstract slot, a base-typed caller reaches each
// override, and a base method calls the abstract method on this.
func TestClassAbstractRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `abstract class Shape {
  name: string;
  constructor(name: string) {
    this.name = name;
  }
  abstract area(): number;
  describe(): string {
    return this.name + ":" + this.area();
  }
}
class Square extends Shape {
  side: number;
  constructor(side: number) {
    super("square");
    this.side = side;
  }
  area(): number {
    return this.side * this.side;
  }
}
class Circle extends Shape {
  r: number;
  constructor(r: number) {
    super("circle");
    this.r = r;
  }
  area(): number {
    return 3 * this.r * this.r;
  }
}
function show(s: Shape): string {
  return s.describe();
}
console.log(show(new Square(3)));
console.log(show(new Circle(2)));
const s: Shape = new Square(4);
console.log(s.area());
`
	got := runProgramGo(t, src)
	want := "square:9\n" +
		"circle:12\n" +
		"16\n"
	if got != want {
		t.Fatalf("abstract dispatch program printed %q, want %q", got, want)
	}
}

// TestClassAbstractNoVTableRuns builds and runs the non-virtual abstract
// chain: the subclass constructs through the base's init with no vtable
// anywhere.
func TestClassAbstractNoVTableRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `abstract class Base {
  label: string;
  constructor(label: string) {
    this.label = label;
  }
  show(): string {
    return this.label;
  }
}
class Kid extends Base {
  constructor() {
    super("kid");
  }
}
console.log(new Kid().show());
`
	got := runProgramGo(t, src)
	if want := "kid\n"; got != want {
		t.Fatalf("abstract base program printed %q, want %q", got, want)
	}
}

// TestClassStringLiteralMemberNames pins slice 4's declaration side: a method
// and a field named by a string literal lower to a struct field and method
// whose Go spelling is the same mangling a bracket read of that name uses, so
// the two agree. A space is not a Go identifier rune, so "my method" mangles to
// MyU20_method; the program reads both back through bracket access and prints
// their values.
func TestClassStringLiteralMemberNames(t *testing.T) {
	const src = `class C {
  "my method"(): number { return 42; }
  "my field": number = 5;
}
const c = new C();
console.log(String(c["my method"]()));
console.log(String(c["my field"]));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "MyU20_method") || !strings.Contains(source, "MyU20_field") {
		t.Errorf("string-literal member names did not mangle to a Go spelling:\n%s", source)
	}
	if got, want := runProgramGo(t, src), "42\n5\n"; got != want {
		t.Fatalf("string-member program printed %q, want %q", got, want)
	}
}

// TestClassComputedConstantMemberName pins that a computed name whose expression
// is a constant string is an ordinary property of that name, not a keyword: a
// ["constructor"] method is a prototype method distinct from the class
// constructor, so calling it returns its own value and NewC still constructs.
func TestClassComputedConstantMemberName(t *testing.T) {
	const src = `class C {
  n: number = 1;
  ["plain"](): number { return 7; }
}
const c = new C();
console.log(String(c.n));
console.log(String(c["plain"]()));
`
	if got, want := runProgramGo(t, src), "1\n7\n"; got != want {
		t.Fatalf("computed-constant-name program printed %q, want %q", got, want)
	}
}

// TestClassComputedNonConstantNameHandsBack pins the honest leftover: a computed
// member name that is not a constant string ([Symbol.toPrimitive] here, a well-known
// symbol key this slice does not lower) names itself in the handback reason rather than
// borrowing the identifier-name phrasing, so what remains after this slice is clear. The
// [Symbol.iterator] and [Symbol.asyncIterator] keys are no longer among them: each
// lowers to its iterator protocol's entry point.
func TestClassComputedNonConstantNameHandsBack(t *testing.T) {
	const src = `class C {
  [Symbol.toPrimitive](): number { return 1; }
}
new C();
`
	reason := renderProgramHandBack(t, src)
	if want := "a computed member name that is not a constant string is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestClassGeneratorMethodLinear pins a generator method whose body is a
// straight-line run of yields lowers to a method returning the running coroutine
// value.NewGen builds, a *value.Gen[T]. A for...of over it pulls the coroutine until
// done, so the sum of the yielded values is what the program prints.
func TestClassGeneratorMethodLinear(t *testing.T) {
	const src = `class C {
  *g(): Generator<number> { yield 1; yield 2; yield 3; }
}
const c = new C();
let sum = 0;
for (const v of c.g()) { sum += v; }
console.log(String(sum));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (c *C) G() *value.Gen[float64]") {
		t.Errorf("generator method did not lower to a *value.Gen coroutine:\n%s", source)
	}
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("generator program printed %q, want %q", got, want)
	}
}

// TestClassGeneratorTwoIterators pins that state lives in the returned closure,
// fresh per call, so two iterations that are live at the same time do not share
// a cursor. Nesting a for...of over the same method inside another keeps both
// iterators alive at once; the full nine-pair product prints only if they are
// independent.
func TestClassGeneratorTwoIterators(t *testing.T) {
	const src = `class C {
  *g(): Generator<number> { yield 1; yield 2; yield 3; }
}
const c = new C();
let out = "";
for (const a of c.g()) {
  for (const b of c.g()) {
    out += String(a) + String(b) + " ";
  }
}
console.log(out);
`
	if got, want := runProgramGo(t, src), "11 12 13 21 22 23 31 32 33 \n"; got != want {
		t.Fatalf("interleaved generators printed %q, want %q", got, want)
	}
}

// TestClassGeneratorYieldInControlFlow pins that a yield inside a loop or branch
// lowers with the coroutine: the goroutine suspends at each yield wherever it sits in
// the control flow, so a yield in a for loop drives the loop one turn per pull. The
// for...of over it prints each yielded value in order.
func TestClassGeneratorYieldInControlFlow(t *testing.T) {
	const src = `class C {
  *g(): Generator<number> { for (let i = 0; i < 3; i++) { yield i; } }
}
const c = new C();
let out = "";
for (const v of c.g()) { out += String(v); }
console.log(out);
`
	if got, want := runProgramGo(t, src), "012\n"; got != want {
		t.Fatalf("control-flow generator printed %q, want %q", got, want)
	}
}

// TestClassGeneratorYieldStarHandsBack pins that a yield* over a non-generator
// iterable in a class generator method keeps the same yield*-specific reason the
// function form gives, not the control-flow one, so the next baseline ranks it
// honestly. A yield* over a generator delegate lowers through the shared YieldFrom
// path; only the non-generator iterable, an array here, still hands back.
func TestClassGeneratorYieldStarHandsBack(t *testing.T) {
	const src = `class C {
  *g(): Generator<number> { yield* [1, 2]; }
}
new C();
`
	if reason, want := renderProgramHandBack(t, src), "a yield* over a non-generator iterable is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestSuperSetterStoreRuns pins that a store through a base set accessor spelled
// with super, super.x = v, routes to the base setter method on the embedded base
// value the way the super read and super method call do, so the write reaches the
// base setter with no virtual dispatch.
func TestSuperSetterStoreRuns(t *testing.T) {
	const src = `class A {
  n: number = 1;
  set x(v: number) { this.n = v; }
}
class B extends A {
  put(v: number): void { super.x = v; }
}
const b = new B();
b.put(5);
console.log(String(b.n));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.A.SetX(") {
		t.Errorf("super setter store did not route to the base setter:\n%s", source)
	}
	if got := runProgramGo(t, src); got != "5\n" {
		t.Errorf("super setter store ran wrong\n got: %q\nwant: %q", got, "5\n")
	}
}

// TestSuperStaticMethodRuns pins that super.m() inside a static method calls the
// base class's static method, which lowered to a package function, so a static
// override reaches the base static the way A.m() does rather than through an
// instance receiver the static body does not have.
func TestSuperStaticMethodRuns(t *testing.T) {
	const src = `class A {
  static make(): number { return 1; }
}
class B extends A {
  static make(): number { return super.make() + 1; }
}
console.log(String(B.make()));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "AMake() + 1") {
		t.Errorf("super static call did not route to the base static function:\n%s", source)
	}
	if got := runProgramGo(t, src); got != "2\n" {
		t.Errorf("super static method ran wrong\n got: %q\nwant: %q", got, "2\n")
	}
}

// TestSuperOverridingAccessorHandsBack pins the boundary: a derived accessor
// overriding a same-named base accessor needs a virtual accessor slot so a
// base-typed read reaches the override, machinery only methods have today, so
// the override hands back rather than dispatch statically to the base accessor.
func TestSuperOverridingAccessorHandsBack(t *testing.T) {
	const src = `class A {
  get v(): number { return 2; }
}
class B extends A {
  get v(): number { return super.v + 3; }
}
new B();
`
	if reason, want := renderProgramHandBack(t, src), "a virtual accessor override is a later slice"; !strings.Contains(reason, want) {
		t.Fatalf("handback reason = %q, want it to contain %q", reason, want)
	}
}
