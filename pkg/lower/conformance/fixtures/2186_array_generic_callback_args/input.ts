const o: any = {};
o[0] = 10;
o[1] = 20;
o[2] = 30;
o.length = 3;

const withIndex: any = Array.prototype.map.call(o, (x: any, i: any) => x + i);
console.log(withIndex[0]);
console.log(withIndex[1]);
console.log(withIndex[2]);

const withObject: any = Array.prototype.map.call(o, (x: any, i: any, arr: any) => x + arr.length);
console.log(withObject[0]);
console.log(withObject[1]);
console.log(withObject[2]);
