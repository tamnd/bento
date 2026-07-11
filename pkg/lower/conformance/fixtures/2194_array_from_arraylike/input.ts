// Array.from reads an array-like value: its length and integer keys, filling a
// missing index with undefined. The source is dynamic, so Array.from walks it as
// an array-like at runtime rather than copying a typed array.
const like: any = {};
like.length = 3;
like[0] = "a";
like[2] = "c";
const arr: any = Array.from(like);
console.log(arr.length);
console.log(arr[0]);
console.log(arr[1]);
console.log(arr[2]);

// The optional map callback rewrites each element from its value and index.
const src: any = [10, 20, 30];
const nums: any = Array.from(src, (v: any, i: any) => v + i);
console.log(nums[0]);
console.log(nums[1]);
console.log(nums[2]);
