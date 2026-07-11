const o: any = {};
o[0] = 1;
o[1] = 2;
o[2] = 3;
o.length = 3;

const doubled: any = Array.prototype.map.call(o, (x: any) => x * 2);
console.log(doubled.length);
console.log(doubled[0]);
console.log(doubled[2]);

const odds: any = Array.prototype.filter.call(o, (x: any) => x % 2 === 1);
console.log(odds.length);
console.log(odds[0]);

const mid: any = Array.prototype.slice.call(o, 1);
console.log(mid.length);
console.log(mid[0]);

const remapped: any = Array.prototype.map.call(mid, (x: any) => x * 10);
console.log(remapped[0]);
console.log(remapped[1]);
