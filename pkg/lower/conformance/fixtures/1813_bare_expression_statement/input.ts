// A bare expression statement evaluates its operand and throws the value away.
// Go has no statement that lets a value stand alone the way a call does, so each
// lowers to the _ = expr discard: a lone identifier, a discarded comparison, a
// discarded arithmetic result, and a discarded conditional all keep their
// evaluation and drop the result.
let x: number = 5;
x;
x < 10;
x + 1;
x > 0 ? x : -x;
console.log(x);
