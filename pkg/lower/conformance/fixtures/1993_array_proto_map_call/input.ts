// Array.prototype.map.call(arrayLike, String) borrows the base array map to
// stringify each element of any array-like, the idiom the test262 assert prelude's
// compareArray.format uses to render a failed comparison. The receiver is the
// function Array.prototype.map, not a value, so the call routes to the
// map-and-stringify helper rather than handing back at the non-string-receiver
// gate, and the dynamic-element result then joins. A string is an array-like too,
// so the borrow indexes it to its characters the way the engine does.

function fmt(arrayLike: any): string {
  return "[" + Array.prototype.map.call(arrayLike, String).join(", ") + "]";
}
console.log(fmt([1, 2, 3]));
console.log(fmt(["a", "b"]));
console.log(fmt([true, false]));
console.log(fmt("xyz"));
