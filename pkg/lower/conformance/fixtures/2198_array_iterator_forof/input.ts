// for...of consumes an array iterator without building the iterator object: the
// values kind binds each element, the keys kind the index as a number, and a
// destructuring [i, v] over the entries kind binds the index and element together.
const a = [10, 20, 30];

let sum = 0;
for (const v of a.values()) {
  sum += v;
}
console.log(sum);

let ksum = 0;
for (const i of a.keys()) {
  ksum += i;
}
console.log(ksum);

let esum = 0;
for (const [i, v] of a.entries()) {
  esum += i * 100 + v;
}
console.log(esum);
