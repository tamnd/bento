// An object destructuring assignment binds already-declared locals from the property
// of the same name. It is parenthesized in statement position since a leading { would
// open a block. Go has no destructuring, so it lowers to a single parallel assignment,
// x, y = o.X, o.Y, the same struct-field selector a written-out property access lowers
// to. The source must be a plain variable of a fixed-shape object, and the targets are
// shorthand names only, so a rename, a default, a rest, or a call source hands back.
const o = { x: 10, y: 20, z: 30 };
let x = 0;
let y = 0;
let z = 0;
({ x, y, z } = o);
console.log(x + y + z);

// The assignment reads the source once per target, so reassigning from a fresh source
// picks up the new values.
const next = { x: 1, y: 2, z: 3 };
({ x, y, z } = next);
console.log(x + y + z);

// The targets can be a subset of the source's properties.
const rec = { label: "sam", age: 30, active: true };
let label = "";
let active = false;
({ label, active } = rec);
console.log(label + " " + active);
