const grid: number[][] = [[1, 2, 3], [4, 5]];
const [[a, ...rest], [b]] = grid;
console.log(a);
console.log(rest.length);
console.log(b);
