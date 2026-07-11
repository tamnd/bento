// Array.of builds an array from its arguments one to one, so a single number is a
// single element. This is where it differs from the Array(n) constructor, whose
// one number argument sets a length rather than storing an element.
const one = Array.of(7);
console.log(one.length);
console.log(one[0]);

const trio = Array.of(1, 2, 3);
console.log(trio.length);
console.log(trio[2]);
