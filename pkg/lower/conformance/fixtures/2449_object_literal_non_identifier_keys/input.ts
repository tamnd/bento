const p = { "a": 1, "b": 2 };
console.log(String(p.a + p.b));
const q = { ["c"]: 9 };
console.log(String(q.c));
const o: any = { "x y": 5, 42: 7 };
console.log(String(o["x y"]));
console.log(String(o[42]));
console.log(String(o["42"]));
