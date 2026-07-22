// An arithmetic operator over a statically typed string or boolean operand is one
// JavaScript runs by coercing each side through ToNumber, but TypeScript rejects the
// operand: 2362 on the left, 2363 on the right ("the left/right-hand side of an
// arithmetic operation must be of type number, bigint, or any"). The ahead-of-time
// path compiles TypeScript, so it honors that rejection and hands the whole unit
// back rather than emit the ToNumber coercion for a program the checker refused. The
// hand-back keys on the checker's 2362/2363 span, so a .js source, which the checker
// does not flag, would keep the coercion once the AOT path admits .js.
let s = "5";
let n = 2;

// "5" - 2: the left operand is a string, a 2362 the whole unit hands back for.
console.log(s - n);

// true * 4: a boolean operand is a 2363, so this too hands the unit back.
let t = true;
console.log(t * 4);

// "x" * 2, "5" % 3, "2" ** 3, and "6" & 3 are each a string operand the checker
// rejects, so no arithmetic, remainder, exponent, or bitwise operator over a string
// escapes the hand-back.
let bad = "x";
console.log(bad * 2);
console.log(s % 3);
let base = "2";
console.log(base ** 3);
let bits = "6";
console.log(bits & 3);
