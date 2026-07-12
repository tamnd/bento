const ta = new Int32Array([10, 20, 30]);
const x: any = ta[100];
console.log(x === undefined);
const y: any = ta[1.5];
console.log(y === undefined);
ta[100] = 999;
ta[1.5] = 999;
console.log(ta[0], ta[1], ta[2]);
console.log(ta.length);
