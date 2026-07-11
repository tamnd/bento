const o: any = {};
o[0] = 1;
o[1] = 2;
o[2] = 3;
o.length = 3;

const doubled: any = Array.prototype.map.call(o, (x: any) => x * 2);
console.log(doubled[0]);
console.log(doubled[1]);
console.log(doubled[2]);

const odds: any = Array.prototype.filter.call(o, (x: any) => x % 2 === 1);
console.log(odds.length);

const anyBig: any = Array.prototype.some.call(o, (x: any) => x > 2);
console.log(anyBig);

const allPos: any = Array.prototype.every.call(o, (x: any) => x > 0);
console.log(allPos);

const found: any = Array.prototype.find.call(o, (x: any) => x === 2);
console.log(found);

const idx: any = Array.prototype.findIndex.call(o, (x: any) => x === 3);
console.log(idx);
