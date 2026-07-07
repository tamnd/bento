// A binding declared with no initializer (var y;) holds undefined until its first
// write, so its Go slot is a boxed value.Value. Control-flow analysis narrows a
// later read to the primitive the writes settle on, so a compound assignment
// y /= -1 reads y as a number and computes a static float64, which does not fit
// the boxed slot. The result is boxed back to the slot: value.Number for an
// arithmetic operator, value.StringValue for a + that concatenates. A test262
// compound-assignment probe drives the numeric shape, var x = 1; x /= -1, and its
// no-initializer sibling var y; y = 1; y /= -1.
var n;
n = 10;
n -= 3;
n *= 2;
n /= 7;
n %= 1.5;
console.log(n);

var s;
s = "a";
s += "b";
s += "c";
console.log(s);
