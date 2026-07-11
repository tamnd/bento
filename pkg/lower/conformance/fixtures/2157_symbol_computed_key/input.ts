const o: any = {};
const s = Symbol("k");
o[s] = 42;
console.log(o[s]);
console.log(delete o[s]);
console.log(o[s]);
