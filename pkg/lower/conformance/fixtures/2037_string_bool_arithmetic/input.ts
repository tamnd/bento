// An arithmetic operator over a statically typed string or boolean operand runs
// at run time by coercing each side through ToNumber first, even though the
// checker rejects the operand (2362 on the left, 2363 on the right). This proves
// the AOT path admits those reports and lowers each operand through its numeric
// coercion, a string through value.StringToNumber and a boolean through
// value.BoolToNumber, rather than refusing the operator.
let s = "5";
let n = 2;

// A numeric string minus a number: "5" - 2 is 3.
console.log(s - n);

// A boolean is 1 when true and 0 when false, so true * 4 is 4.
let t = true;
console.log(t * 4);

// A string ToNumber cannot parse yields NaN, so "x" * 2 is NaN, not a crash.
let bad = "x";
console.log(bad * 2);

// Remainder and exponent reach the coercion too, lowering to math.Mod and
// value.Pow: "5" % 3 is 2 and "2" ** 3 is 8.
console.log(s % 3);
let base = "2";
console.log(base ** 3);

// A bitwise operator coerces the string operand to int32 the same way a number
// operand is coerced: "6" & 3 is 2.
let bits = "6";
console.log(bits & 3);
