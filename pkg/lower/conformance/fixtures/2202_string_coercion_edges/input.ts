// String() called with no argument is the empty string, the coercion-edge count
// the spec fixes: with no value to convert the result is "", not "undefined". The
// primitive coercions read exactly as the engine's ToString does.
console.log(String() === "");
console.log(String(123));
console.log(String(true));
console.log(String(-0));
console.log(String(1.5));
