// A parenthesized string ternary coerces to the string it is. The grouping
// parentheses a concat operand, a bare log argument, and a member receiver carry
// wrap the conditional in a parenthesized node whose type is still the branches'
// literal union, so isString sees through the parentheses to the ternary within.
// This clears the coercion, the + concatenation, and the .length read, which a
// member access on a ternary receiver always parenthesizes.
function show(x: number): void {
  console.log((x > 0 ? "hi" : "yo"));
  console.log((x > 0 ? "hi" : "yo") + "!");
  console.log("v=" + (x > 0 ? "hi" : "yo"));
  console.log((x > 0 ? "hi" : "yo").length);
}

show(1);
show(-1);
