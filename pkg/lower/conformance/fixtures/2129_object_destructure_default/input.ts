const pt: { x: number; y?: number } = { x: 1 };
const { x, y = 5 } = pt;
console.log(x);
console.log(y);
