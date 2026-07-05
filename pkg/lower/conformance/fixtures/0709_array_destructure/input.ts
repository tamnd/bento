// An array destructuring binding names each element by position. Go has no
// destructuring, so it lowers to one short declaration per element reading through
// the array's positional AtI, the same read a written-out element access lowers to.
// The source must be a plain variable, so the read repeats without evaluating it
// once per element, and the pattern is flat names whose types match the element
// type. Holes, defaults, rest, and nested patterns are later slices.
const nums: number[] = [10, 20, 30];
const [a, b, c] = nums;
console.log(a + b + c);

const names: string[] = ["alice", "bob"];
const [first, second] = names;
console.log(first + " and " + second);

const single: number[] = [7];
const [only] = single;
console.log(only);
