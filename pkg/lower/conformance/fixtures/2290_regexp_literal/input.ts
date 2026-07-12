// A regexp literal and an equivalent RegExp constructor both build a RegExp, a value
// whose typeof is "object" and whose Object.prototype.toString tag is "[object
// RegExp]". These reads exercise the RegExp value the two forms lower to before the
// flag, exec, and match slices give it observable behavior.
const re = /ab+c/;
console.log(typeof re === "object");
console.log(Object.prototype.toString.call(re));
const ctor = new RegExp("ab+c");
console.log(typeof ctor === "object");
console.log(Object.prototype.toString.call(ctor));
