// The ||= and &&= logical assignments short-circuit on the target's JavaScript
// truthiness, not just a boolean. A zero and an empty string are falsy, so ||=
// fills them and &&= leaves them; a non-zero number and a non-empty string are
// truthy, so ||= leaves them and &&= replaces them. A dynamic target reads the
// same falsy set at runtime. The assignment runs only when the guard fires, so
// the fallback is never stored over a value that already holds.
let n = 0;
n ||= 5;
console.log(n);

let m = 3;
m ||= 9;
console.log(m);

let s = "";
s ||= "filled";
console.log(s);

let word = "kept";
word &&= "changed";
console.log(word);

let z = 0;
z &&= 8;
console.log(z);

function orDyn(x: any): number { x ||= 7; return x; }
console.log(orDyn(0));
console.log(orDyn(2));
