// Array.isArray brands a real array true and everything else false, including an
// array-like object that only carries a length.
console.log(Array.isArray([1, 2, 3]));

const like: any = {};
like.length = 3;
console.log(Array.isArray(like));

const boxed: any = [1, 2];
console.log(Array.isArray(boxed));

console.log(Array.isArray("nope"));
console.log(Array.isArray(42));
