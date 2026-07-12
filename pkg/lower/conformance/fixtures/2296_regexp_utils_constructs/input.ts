// String.fromCodePoint and String.fromCharCode are variadic, and regExpUtils.js's
// buildString invokes them through .apply over a code-point array of unknown length. The
// runtime array spreads into the variadic value constructor, so the string is built
// without a spread literal, and .call spells its arguments inline past the dropped this.
function stringFromRange(start: number, end: number): string {
  const codePoints: number[] = [];
  for (let cp = start; cp <= end; cp++) {
    codePoints.push(cp);
  }
  return String.fromCodePoint.apply(null, codePoints);
}
console.log(stringFromRange(97, 99)); // abc, apply over a runtime array
console.log(String.fromCodePoint.apply(null, [65, 66, 67])); // ABC, apply over a literal
console.log(String.fromCharCode.call(null, 88, 89, 90)); // XYZ, call past the this
console.log(String.fromCodePoint.apply(null, [])); // empty string, apply over []

// printCodePoint from regExpUtils.js formats a code point as U+XXXXXX, and its chain of
// number and string methods lowers unchanged.
function printCodePoint(codePoint: number): string {
  const hex = codePoint.toString(16).toUpperCase().padStart(6, "0");
  return `U+${hex}`;
}
console.log(printCodePoint(97)); // U+000061
console.log(printCodePoint(0x1f600)); // U+01F600
