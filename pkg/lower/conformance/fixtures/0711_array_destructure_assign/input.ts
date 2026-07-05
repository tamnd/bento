// An array destructuring assignment assigns already-declared targets from the right
// side by position. It lowers to a single Go parallel assignment, which evaluates
// every right-hand side before assigning any target. That order is what makes the
// swap idiom fall out as Go's own parallel swap. The source is a plain array variable,
// read element by element, or an array literal of the same arity.
let a = 1;
let b = 2;
const pair: number[] = [10, 20];
[a, b] = pair;
console.log(a + " " + b);

[a, b] = [b, a];
console.log(a + " " + b);

let x = 1;
let y = 2;
let z = 3;
[x, y, z] = [z, x, y];
console.log(x + " " + y + " " + z);
