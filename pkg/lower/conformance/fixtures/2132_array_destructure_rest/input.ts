const arr: number[] = [10, 20, 30, 40];
const [a, b, ...rest] = arr;
console.log(a);
console.log(b);
console.log(rest.length);
console.log(rest[0]);
console.log(rest[1]);
