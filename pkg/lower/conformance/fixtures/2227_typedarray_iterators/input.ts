// A numeric typed array is its own iterable: for...of yields each element as a
// Number, and values, keys, and entries range the view the same way an array's do.
// A for...of over one of these ranges the view directly rather than build and drive
// an iterator object, the idiomatic loop a developer writes.
const a = new Int32Array([10, 20, 30]);

// for...of ranges the elements as Numbers.
let sum = 0;
for (const x of a) {
  sum += x;
}
console.log(sum);

// values() ranges the same elements.
let prod = 1;
for (const v of a.values()) {
  prod *= v;
}
console.log(prod);

// keys() ranges the indices.
let ksum = 0;
for (const i of a.keys()) {
  ksum += i;
}
console.log(ksum);

// entries() ranges [index, value] pairs.
for (const [i, v] of a.entries()) {
  console.log(i + ":" + v);
}
