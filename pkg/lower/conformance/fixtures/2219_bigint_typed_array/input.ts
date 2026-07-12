const a = new BigInt64Array(3);
a[0] = 42n;
a[1] = -7n;
a[2] = 9223372036854775808n;
console.log(a[0], a[1], a[2]);
console.log(a.length, a.BYTES_PER_ELEMENT, a.byteLength);

const buf = new ArrayBuffer(16);
const s = new BigInt64Array(buf);
const u = new BigUint64Array(buf);
s[0] = -1n;
console.log(u[0]);
u[1] = 9223372036854775808n;
console.log(s[1]);

const lit = new BigInt64Array([1n, 2n, 3n]);
console.log(lit[0], lit[1], lit[2]);

const x: any = s[100];
console.log(x === undefined);
