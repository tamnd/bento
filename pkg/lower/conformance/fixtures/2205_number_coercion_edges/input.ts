// Number() called with no argument is +0, the coercion-edge count the spec fixes:
// with no value to convert the result is zero, not NaN. The rest read exactly as
// the engine's ToNumber does: a boolean is one or zero, a string trims surrounding
// whitespace before parsing, an empty string is zero, and a hex prefix parses in
// base sixteen.
console.log(Number() === 0);
console.log(Number(true));
console.log(Number(false));
console.log(Number("  42  "));
console.log(Number(""));
console.log(Number("0x1F"));
