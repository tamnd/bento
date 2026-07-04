// JavaScript lets a loop body be a single unbraced statement, not only a braced
// block. A for...of and a while accept that form the same way a for and an if
// already do: the lone statement lowers on its own and is wrapped in the Go block
// the loop requires. So `for (const x of xs) sum += x;` and `while (c) s++;` lower
// straight through instead of handing the unit back to the interpreter.
const xs = [1, 2, 3, 4];
let sum = 0;
for (const x of xs) sum = sum + x;
console.log(String(sum));

const sq: number[] = [];
for (const i of [1, 2, 3]) sq.push(i * i);
console.log(sq.join(","));

let n = 5;
while (n > 0) n = n - 1;
console.log(String(n));

let s = 0;
while (s < 3) s++;
console.log(String(s));
