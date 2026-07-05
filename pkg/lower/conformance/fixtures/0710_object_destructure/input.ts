// An object destructuring binding names each property by its shorthand name. Go has
// no destructuring, so it lowers to one short declaration per property reading through
// the struct-field selector, the same read a written-out property access lowers to.
// The source must be a plain variable of a fixed-shape object, and the pattern is
// shorthand names only; a rename, a default, a rest, or a nested pattern is a later
// slice.
const pt = { x: 10, y: 20 };
const { x, y } = pt;
console.log(x + y);

const rec = { label: "sam", age: 30, active: true };
const { label, age, active } = rec;
console.log(label + " " + age);
console.log(active);
