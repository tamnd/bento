const o = { a: 0 };
const s = { a: 5 };
({ a: o.a } = s);
console.log(o.a);
