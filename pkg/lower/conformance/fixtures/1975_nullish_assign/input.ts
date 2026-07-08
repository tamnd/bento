// The ??= operator stores its right-hand side only when the target is nullish, and
// nullish means null or undefined, never a falsy zero or empty string. A dynamic
// target tests both null and undefined at runtime, so a present value of any kind is
// left alone. An optional local takes a definite fallback and narrows to the plain
// value afterward, so the arithmetic that follows sees a number rather than an option.

function fill(v: any): any {
  v ??= "def";
  return v;
}
console.log(fill(undefined));
console.log(fill(null));
console.log(fill(0));
console.log(fill(""));
console.log(fill("set"));

let a: number | undefined = undefined;
a ??= 5;
console.log(a + 1);

let b: number | undefined = 10;
b ??= 99;
console.log(b + 1);
