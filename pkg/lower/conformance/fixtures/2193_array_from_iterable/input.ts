// Array.from over a real array copies it: the result holds the same elements but
// is a distinct array, so writing the copy leaves the source untouched.
const src = [1, 2, 3];
const copy = Array.from(src);
console.log(copy.length);
copy[0] = 9;
console.log(src[0]);
console.log(copy[0]);

// Array.from over a string splits it into one substring per code point.
const chars = Array.from("hi");
console.log(chars.length);
console.log(chars[1]);
