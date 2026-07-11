const arr: { x: number; y: number }[] = [{ x: 1, y: 2 }, { x: 3, y: 4 }];
const [{ x, y }, { x: p, y: q }] = arr;
console.log(x);
console.log(y);
console.log(p);
console.log(q);
