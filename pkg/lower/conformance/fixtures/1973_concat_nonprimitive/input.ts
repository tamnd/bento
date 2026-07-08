// The + operator is string concatenation as soon as one side is a string, and it
// coerces a non-primitive operand through ToPrimitive then ToString the same way the
// value model does: an object literal reports the [object Object] tag and an array
// literal joins its elements with commas. This holds whichever side the non-primitive
// is on.

console.log("obj=" + { a: 1 });
console.log("arr=" + [1, 2, 3]);
console.log([10, 20] + "!");
console.log("nested=" + { a: 1 } + "/" + [4, 5]);
