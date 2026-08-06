// A tagged-sum union keeps its value in the field its tag selects, and those fields are
// unexported, so the JSON walk reads the union through a JSONArm method the renderer
// emits. A sentinel arm has no field to hand over, and it used to hand over a bare nil,
// which the walk read as nothing at all rather than as a value: an array of unions
// holding a null wrote "[0,,1]" and an object field wrote a key with nothing after it,
// neither of which is JSON.
//
// It hands over the singleton now, value.Null or value.Undefined, and the walk places
// each the way JSON.stringify does: null renders as null wherever it appears, and
// undefined folds to null in an array and omits its key in an object.
//
// An absent optional reaching the walk in one of those positions is the same question,
// so it is here too.

function pickNull(n: number): number | null {
  return n > 1 ? null : n;
}

function pickUndef(n: number): number | string | undefined {
  if (n > 2) return "s";
  if (n > 1) return undefined;
  return n;
}

const nulls = [pickNull(0), pickNull(5), pickNull(1)];
const undefs = [pickUndef(0), pickUndef(2), pickUndef(3)];
const opts = [1, 2, 3].map((n): number | undefined => (n === 2 ? undefined : n));

console.log(JSON.stringify(nulls));
console.log(JSON.stringify(undefs));
console.log(JSON.stringify(opts));

console.log(JSON.stringify({ a: pickNull(5), b: pickNull(0) }));
console.log(JSON.stringify({ a: pickUndef(2), b: pickUndef(0) }));

console.log(JSON.stringify(pickNull(5)), JSON.stringify(pickNull(0)));

// The indented walk takes the same two arms, so a gap does not change which values
// render.
console.log(JSON.stringify(nulls, null, 2));
console.log(JSON.stringify({ a: pickUndef(2), b: pickUndef(3) }, null, 1));

// And the values still read as themselves outside JSON.
console.log(nulls, undefs, opts);
