const o: { x: number; y?: number } = { x: 1 };
const { x: a, y: b = 9 } = o;
console.log(a);
console.log(b);
