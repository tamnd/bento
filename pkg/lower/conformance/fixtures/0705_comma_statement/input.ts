// A comma expression in statement position evaluates its operands left to right
// and throws the result away, so it lowers to the Go statements those operands
// spell, one per operand. A person writes it to sequence a few assignments or a
// few calls on one line. The operands run in order, which the running total and
// the order of the logged lines both show.
let a = 0;
let b = 0;
let c = 0;
a = 1, b = 2, c = 3;
console.log(a + b + c);

let step = 0;
step = step + 1, step = step * 10, step = step + 5;
console.log(step);

console.log("x"), console.log("y");
