// A multi-member primitive union plus undefined or null lowers to the tagged-sum
// struct with a tag-only arm for each sentinel: no payload field, a no-argument
// constructor, and a tag that tells it apart from the value arms. undefined and null
// are single values, so the tag alone stands for them, and every narrowing the source
// writes, x === undefined, x === null, and typeof, reads that tag. This fixture drives
// a number | string | undefined through a presence test and a typeof narrowing and a
// number | string | null through its value and null arms.

function classify(x: number | string | undefined): string {
  if (x === undefined) return "missing";
  if (typeof x === "number") return "num:" + x;
  return "str:" + x;
}

console.log(classify(7));
console.log(classify("hi"));
console.log(classify(undefined));

let slot: number | string | null = 3;
// A bare read prints the value narrowed to its arm.
console.log(slot);
slot = "text";
console.log(slot);
// A reassignment to the null arm, then the presence tests over it.
slot = null;
console.log(slot === null);
console.log(slot !== null);
// typeof a null-arm value is "object", the value JavaScript reports for null.
console.log(typeof slot);
