// A compound assignment other than + on a dynamic (boxed) local used to hand back:
// combineBinary already coerced the boxed target through ToNumber and ran the native
// operator, leaving a float64, but the float64 does not fit the value.Value slot, so
// the store needs value.Number to box it back. Every arithmetic and bitwise operator
// takes this shape; a + on a dynamic target already lowered through StringValue or
// value.Add.
let x: any = 12;
x -= 3;
x *= 2;
x /= 2;
x %= 4;
x **= 5;
console.log(x);
let y: any = 6;
y &= 3;
y |= 8;
y ^= 1;
y <<= 2;
y >>= 1;
console.log(y);
const r: number = (y -= 2);
console.log(r);
console.log(y);
