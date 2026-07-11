const a: any = [1, 2, 3];
delete a[1];

// reduce folds only the present elements, so the hole at index 1 is skipped.
const sum: any = Array.prototype.reduce.call(a, (acc: any, x: any) => acc + x);
console.log(sum);

// counting visits shows the hole is never handed to the callback.
const count: any = Array.prototype.reduce.call(a, (acc: any, _x: any) => acc + 1, 0);
console.log(count);

// a stored undefined is present, so it is visited and the count is one higher.
const b: any = [1, undefined, 3];
const bcount: any = Array.prototype.reduce.call(b, (acc: any, _x: any) => acc + 1, 0);
console.log(bcount);

// indexOf reports -1 for a value that only lived at the deleted index.
const gone: any = Array.prototype.indexOf.call(a, 2);
console.log(gone);
const kept: any = Array.prototype.indexOf.call(a, 3);
console.log(kept);

// reduceRight folds right to left over the present elements: 3 - 1.
const diff: any = Array.prototype.reduceRight.call(a, (acc: any, x: any) => acc - x);
console.log(diff);
