const nums = [1, 2, 300];
const fromArr = new Int32Array(nums);
console.log(fromArr[0], fromArr[1], fromArr[2]);

const src = new Int32Array([1, 2, 300]);
const asBytes = new Uint8Array(src);
console.log(asBytes[0], asBytes[1], asBytes[2]);

const asF64 = new Float64Array(src);
console.log(asF64[2]);

src[0] = 99;
console.log(asBytes[0]);
