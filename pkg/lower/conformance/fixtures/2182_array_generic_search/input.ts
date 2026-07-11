const o: any = {};
o[0] = "a";
o[1] = "b";
o[2] = "a";
o.length = 3;

const i0: any = Array.prototype.indexOf.call(o, "a");
console.log(i0);
const i1: any = Array.prototype.indexOf.call(o, "a", 1);
console.log(i1);
const i2: any = Array.prototype.indexOf.call(o, "z");
console.log(i2);
const l0: any = Array.prototype.lastIndexOf.call(o, "a");
console.log(l0);
const c0: any = Array.prototype.includes.call(o, "b");
console.log(c0);
const c1: any = Array.prototype.includes.call(o, "z");
console.log(c1);
