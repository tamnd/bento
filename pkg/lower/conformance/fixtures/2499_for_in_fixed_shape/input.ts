const o = { a: 1, b: 2, c: 3 };
for (const k in o) console.log(k);

let n = 0;
for (const _k in o) n++;
console.log("count", n);

const p = { x: 10, y: 20 };
const keys: string[] = [];
for (const k in p) keys.push(k);
console.log(keys.join(","));

const rec: Record<string, number> = { d: 4, e: 5 };
rec.f = 6;
for (const k in rec) console.log(k, rec[k]);
