// A constant modulus that evaluates to zero, used as a divisor. JavaScript
// gives Infinity, where a naive lowering emits a Go constant division by zero
// the compiler rejects. The fold sees through the float64 cast the remainder
// path wraps its Go % in and evaluates the modulus, so the divide folds to the
// infinity the language yields.
console.log(1 / (1 % 1));
console.log(1 / (0 % 1));
console.log(-1 / (1 % 1));
console.log(1 % 1 === 0);
