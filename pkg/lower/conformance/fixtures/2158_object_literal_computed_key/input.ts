const k = "key";
const s = Symbol("s");
const o: any = { [k]: 1, [s]: 2, plain: 3 };
console.log(o[k]);
console.log(o[s]);
console.log(o.plain);
