const o = { a: 0, b: 0 };
const xs: number[] = [1, 2];
[o.a, o.b] = xs;
console.log(o.a);
console.log(o.b);
