const arr: number[] = [5, 6, 7, 8];
let a = 0;
let rest: number[] = [];
[a, ...rest] = arr;
console.log(a);
console.log(rest.length);
console.log(rest[0]);
console.log(rest[2]);
