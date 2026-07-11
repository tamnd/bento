const grid: number[][] = [[1], [3, 4]];
const [[a, b = 9], [c, d]] = grid;
console.log(a);
console.log(b);
console.log(c);
console.log(d);
