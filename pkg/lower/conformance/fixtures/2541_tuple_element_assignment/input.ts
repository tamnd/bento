// A tuple's positions carry their own types, so its Go shape is a positional struct and
// a write to a position is an assignment to the field its read already selects. The
// read half needed no question about who else holds the array, because a read cannot
// tell a copy from the original. A write can, which is what the guards here are about.

const t: [string, number, boolean] = ["a", 2, false];
t[0] = t[0] + "b";
t[1] = t[1] * 3;
t[2] = !t[2];
console.log("write", t[0], t[1], t[2], t.length);

// The compound spellings desugar to the read, the operator, and the store, all naming
// the one field, so a += on a string position concatenates and a += on a number adds.
t[1] += 4;
t[1] -= 1;
t[1] *= 2;
console.log("compound", t[1]);
t[0] += "!";
console.log("concat", t[0]);

// A step discards its result in statement position, so the prefix and postfix forms are
// the same store and both are the Go step on the field.
t[1]++;
++t[1];
t[1]--;
console.log("step", t[1]);

// A write does not have to come from a literal. A function whose every return answers
// an array literal mints a fresh array, which is how the array a program writes to
// usually arrives, so a binding initialized from one can be written through.
function mk(n: number): [number, string] {
  if (n > 0) {
    return [n, "pos"];
  }
  return [0, "zero"];
}
const p = mk(5);
p[0] = p[0] + 1;
p[1] = p[1].toUpperCase();
console.log("factory", p[0], p[1]);

// Spreading a tuple splices its positions into the call rather than hand the array
// over, so it is a use that keeps the array where it is and the write above stands.
function join(a: number, b: string): string {
  return a + ":" + b;
}
console.log("spread", join(...p));

// A write reads the whole right-hand side before it stores, so a position can be
// computed from the tuple's own other positions.
const q: [number, number] = [1, 2];
q[1] = q[0] + q[1];
q[0] = q[1] - q[0];
console.log("selfref", q[0], q[1]);

// The write lands on the position the index names and leaves the others alone.
const r: [number, number, number] = [10, 20, 30];
r[1] = 99;
console.log("one-slot", r[0], r[1], r[2]);
