const a: any = { x: 1 };
const b: any = {};
Object.setPrototypeOf(b, a);
console.log(b.x);
console.log(Object.getPrototypeOf(b) === a);
Object.setPrototypeOf(b, null);
console.log(b.x);
console.log(Object.getPrototypeOf(b) === null);
