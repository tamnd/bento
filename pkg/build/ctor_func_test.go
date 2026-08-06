package build

import "testing"

// The ES5 constructor function end to end. The unit tests in pkg/value pin the runtime
// value and the ones in pkg/lower pin what the compiler emits; these run the compiled
// program and compare its output to what node 24 prints for the same source, which is
// the only check that says the whole idiom works rather than each half of it.

// TestES5ConstructorFunctionIdiom runs the four lines the idiom is made of, over the two
// levels of prototype chain Node's helpers use: a base constructor with a method on its
// prototype, a derived one that chains through call and links its prototype to the
// base's, and an instance that reads its own fields through an inherited method.
func TestES5ConstructorFunctionIdiom(t *testing.T) {
	src := `"use strict";
function Shape(name) {
  this.name = name;
  this.sides = 0;
}
Shape.prototype.describe = function () {
  return this.name + ":" + this.sides;
};

function Square(size) {
  Shape.call(this, "square");
  this.sides = 4;
  this.size = size;
}
Object.setPrototypeOf(Square.prototype, Shape.prototype);
Square.prototype.area = function () {
  return this.size * this.size;
};

const sq = new Square(3);
console.log(sq.describe());
console.log(sq.area());
console.log(sq.name, sq.sides, sq.size);
console.log(sq instanceof Square, sq instanceof Shape, {} instanceof Shape);
console.log(typeof Shape, Shape.name, Shape.length);
console.log(Object.getPrototypeOf(sq) === Square.prototype);
console.log(Square.prototype.constructor === Square);
console.log(Object.keys(Shape).length);
`
	want := "square:4\n9\nsquare 4 3\ntrue true false\nfunction Shape 1\ntrue\ntrue\n0\n"
	if got := buildAndRunFile(t, "main.js", src); got != want {
		t.Errorf("want:\n%s\ngot:\n%s", want, got)
	}
}

// TestES5ConstructorReplacedPrototype pins the other way a program builds the chain:
// assigning a whole new object over .prototype rather than relinking the one the
// constructor was born with. Construct reads .prototype at construction time, so an
// instance built after the replacement links to the new object and one built before
// keeps the old, which is what the language does.
func TestES5ConstructorReplacedPrototype(t *testing.T) {
	src := `"use strict";
function Base() { this.kind = "base"; }
Base.prototype.who = function () { return "base:" + this.kind; };

function Derived() { Base.call(this); this.kind = "derived"; }
Derived.prototype = new Base();
Derived.prototype.constructor = Derived;

const d = new Derived();
console.log(d.who());
console.log(d instanceof Derived, d instanceof Base);
console.log(Derived.prototype.constructor === Derived);
`
	want := "base:derived\ntrue true\ntrue\n"
	if got := buildAndRunFile(t, "main.js", src); got != want {
		t.Errorf("want:\n%s\ngot:\n%s", want, got)
	}
}

// TestES5ConstructorThroughRequire is the shape the idiom actually arrives in. Node's
// test/common/arraystream.js declares a constructor, links its prototype to the one it
// requires, and exports it; every repl test reaches it that way. The constructor is a
// loader local rather than a package var here, and the requiring module knows nothing
// about it beyond that it holds a value, so new and instanceof both go through the
// runtime rather than through anything the checker resolved.
func TestES5ConstructorThroughRequire(t *testing.T) {
	files := map[string]string{
		"stream.js": `"use strict";
function Stream() { this.kind = "stream"; }
Stream.prototype.describe = function () { return "S:" + this.kind; };
module.exports = Stream;
`,
		"arraystream.js": `"use strict";
const Stream = require("./stream.js");
function ArrayStream() { Stream.call(this); this.rows = []; }
Object.setPrototypeOf(ArrayStream.prototype, Stream.prototype);
ArrayStream.prototype.readable = true;
ArrayStream.prototype.write = function (row) { this.rows.push(row); return this.rows.length; };
module.exports = ArrayStream;
`,
		"main.js": `"use strict";
const ArrayStream = require("./arraystream.js");
const s = new ArrayStream();
console.log(s.describe());
console.log(s.readable, s.kind);
console.log(s.write("a"), s.write("b"));
console.log(s instanceof ArrayStream);
`,
	}
	want := "S:stream\ntrue stream\n1 2\ntrue\n"
	if got := buildAndRun(t, "main.js", files); got != want {
		t.Errorf("want:\n%s\ngot:\n%s", want, got)
	}
}

// TestES5ConstructorCalledWithoutNew pins the guard the idiom opens with, which is why
// instanceof over a constructor had to work at all: a constructor called as a plain
// function detects it and constructs properly instead of writing onto undefined.
func TestES5ConstructorCalledWithoutNew(t *testing.T) {
	src := `"use strict";
function Point(x) {
  if (!(this instanceof Point)) { return new Point(x); }
  this.x = x;
}
Point.prototype.get = function () { return this.x; };

console.log(Point(7).get());
console.log(new Point(8).get());
console.log(Point(9) instanceof Point);
`
	want := "7\n8\ntrue\n"
	if got := buildAndRunFile(t, "main.js", src); got != want {
		t.Errorf("want:\n%s\ngot:\n%s", want, got)
	}
}
