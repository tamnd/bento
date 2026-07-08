// The numeric unary operators +, -, and ~ coerce a non-number operand through
// ToNumber before they run: a numeric string parses to its number, surrounding
// whitespace trims away, a non-numeric string is NaN, and a boolean maps to one or
// zero. Bitwise NOT then complements the 32-bit integer of that coerced number. The
// coercion is the same one the language runs, so the results match a browser.

console.log(+"5");
console.log(-"5");
console.log(~"5");
console.log(+true);
console.log(~true);
console.log(+"3.14");
console.log(-"  10  ");
console.log(+"abc");
