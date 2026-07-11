const o = { x: 10, y: 20 };
let a = 0;
let b = 0;
({ x: a, y: b } = o);
console.log(a);
console.log(b);
