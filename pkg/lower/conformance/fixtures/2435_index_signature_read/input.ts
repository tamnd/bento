// A pure string-index dictionary is a boxed value at run time, so a read by a
// compile-time-constant key routes through the runtime Get and unboxes to the
// signature's element type rather than folding to a fixed-shape miss. Reading a
// key the dictionary carries returns its value, the last piece of the pattern the
// write and the computed-key read already lowered.
type Dict = { [k: string]: number };
const o: Dict = { a: 1 };
o["b"] = 2;
console.log(String(o["a"]));
console.log(String(o["b"]));
