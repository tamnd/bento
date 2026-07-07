// Division by a constant zero folds to a Go constant expression the compiler rejects
// as a division by zero, where JavaScript evaluates it under IEEE rules at runtime.
// A positive numerator over zero is Infinity, a negative one is -Infinity, and zero
// over zero is NaN, so each expression lowers to that value directly and the emitted
// Go compiles. The value package's named numeric constants fold the same way, so a
// divisor like -1 / Number.MAX_VALUE + 1 / Number.MAX_VALUE that cancels to zero is
// seen before the outer divide trips the compiler. The test262 addition and division
// number tests reach for this to name the signed-zero and overflow boundaries.
const pos = 1 / 0;
const neg = -1 / 0;
const nan = 0 / 0;
const cancel = 1 / (-1 / Number.MAX_VALUE + 1 / Number.MAX_VALUE);
const overflow = Number.MAX_VALUE + Number.MAX_VALUE;
console.log(String(pos));
console.log(String(neg));
console.log(String(nan));
console.log(String(cancel));
console.log(String(overflow));
