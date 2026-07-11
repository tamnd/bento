const target: any = { a: 1 };
const result: any = Object.assign(target, { b: 2 }, { a: 9, c: 3 });
console.log(result === target);
console.log(target.a);
console.log(target.b);
console.log(target.c);

const withNullish: any = {};
Object.assign(withNullish, null, { x: 5 }, undefined);
console.log(withNullish.x);
