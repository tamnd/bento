// Audit wave W3: a valueOf() call on a dynamic receiver dispatches at run time the way
// toString already does. Object.prototype.valueOf returns the receiver itself, and the
// primitive wrappers return the primitive they box, so valueOf on a number, boolean, or
// string reads back the same value, and the boxed result flows on into a following
// operation. undefined and null carry no prototype, so reading valueOf off them throws.

const n: any = 42;
const b: any = true;
const s: any = "hi";

console.log(n.valueOf());
console.log(b.valueOf());
console.log(s.valueOf());

// The boxed valueOf result is a usable value: read the number back out and add to it.
const doubled = (n.valueOf() as number) * 2;
console.log(doubled);

// valueOf off undefined throws a TypeError the same way a member read does.
const u: any = undefined;
try {
  u.valueOf();
  console.log("no throw");
} catch (e) {
  console.log(e instanceof TypeError);
}
