// The two loosenesses this group lands, composed: a comma expression evaluated
// for its effect and a call that passes more arguments than the callee accepts.
// The comma runs its left for effect and yields its right, and the extra literal
// arguments the callee never declares are dropped, so the program runs where a
// stricter reading would have refused it. This locks both forms together.
function scale(a: number, b: number): number {
  return a * b;
}

let calls = 0;
let seed = ((calls = calls + 1), 3);
let result = scale(seed, 4, 999, "ignored");
console.log(calls);
console.log(result);

let step = 0;
let total = ((step = step + 10), (step = step + 5), scale(step, 2, 0));
console.log(step);
console.log(total);
