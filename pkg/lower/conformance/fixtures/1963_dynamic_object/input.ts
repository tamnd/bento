const o: any = new Object();
o.a = 1;
o.b = "hi";
console.log(o.a, o.b);

const p: any = new Object();
const q: any = new Object();
p.n = 1;
q.n = 1;
console.log(p.n + q.n);

const r: any = new Object();
r.count = 0;
r.count = r.count + 5;
console.log(r.count);
console.log(r.missing);
