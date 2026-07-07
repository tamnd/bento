// A toString() call on a dynamic receiver dispatches at run time: the value
// carries its own kind, so recv.ToStringMethod() runs the toString that kind's
// prototype installs. A number spells its digits, a boolean spells true or
// false, and a string is itself. This is the read compareArray makes when it
// formats an any-typed message with message.toString() before reporting a
// mismatch.
let n: any = 42;
let b: any = false;
let s: any = "hi";
console.log(n.toString());
console.log(b.toString());
console.log(s.toString());
