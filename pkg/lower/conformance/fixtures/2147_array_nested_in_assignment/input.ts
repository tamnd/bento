const m: number[][] = [[1, 2], [3, 4]];
let a = 0, b = 0, c = 0, d = 0;
([[a, b], [c, d]] = m);
console.log(a);
console.log(b);
console.log(c);
console.log(d);
