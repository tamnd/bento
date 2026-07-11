const o: { p: number[]; q: number[] } = { p: [1, 2], q: [3, 4] };
const { p: [a, b], q: [c, d] } = o;
console.log(a);
console.log(b);
console.log(c);
console.log(d);
