// The ES5 constructor function is how JavaScript wrote classes before it had them, and
// it is how Node's own library and its whole test/common helper tree are still written.
// A plain function declaration lowers to a Go func, which has no .prototype to read, no
// bag to write one onto, no receiver for its body's this, and nothing new can be applied
// to, so every line of the idiom used to hand the unit back.
//
// The unit walks the four lines that make it up over two levels of chain: a base
// constructor, a method on its prototype, a derived constructor that chains through
// call, and Object.setPrototypeOf linking the two prototypes. It then reads back the
// parts a program actually depends on: an inherited method reading the instance's own
// fields, instanceof both ways, the function surface (typeof, name, length), the
// identity of the prototype an instance links to, and the constructor back-pointer.
// Object.keys of the constructor is empty because prototype, name and length are all
// non-enumerable, which is the detail an inspection or a JSON round trip would expose.
"use strict";

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
