// A compound assignment on a member or element target read for its value,
// (o.k += v) / (o[k] += v) / (c.field += v), used to hand back: assignValueCompound
// only lowered an identifier target. It now reuses the statement store and re-reads
// the target inside an immediately-called closure, since bento objects carry no
// getters so the re-read returns exactly what was stored. A side-effecting receiver
// or key still hands back, because the store and the read-back both evaluate it.
const o: any = { n: 10 };
const a = (o.n += 5);
console.log(a);
console.log(o.n);
const key = "n";
const b: number = (o[key] *= 2);
console.log(b);
console.log(o[key]);
const p = { c: 4 };
const d = (p.c -= 1);
console.log(d);
class C { x = 100; }
const inst = new C();
const e = (inst.x /= 4);
console.log(e);
const s: any = { t: "a" };
const g = (s.t += "b");
console.log(g);
