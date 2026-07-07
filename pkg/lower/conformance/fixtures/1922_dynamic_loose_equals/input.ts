// Loose equality over dynamic operands runs the Abstract Equality Comparison,
// which coerces across kinds before it compares: a number against a string reads
// the string as a number, the empty string is 0, and != is the same comparison
// negated. null and undefined are covered in the value-model unit test, where the
// literals need no boxing.
function eq(a: any, b: any): boolean { return a == b; }
function ne(a: any, b: any): boolean { return a != b; }

console.log(eq(1, 1));
console.log(eq(1, "1"));
console.log(eq(0, ""));
console.log(eq("1", 1));
console.log(ne(1, 2));
console.log(ne("a", "b"));
console.log(eq(1, 2));
console.log(eq("a", "b"));
console.log(ne(1, 1));
console.log(eq("", 1));
