// A ++ or -- on a dynamic target runs ToNumeric on the value and adds or
// subtracts one. Every kind but bigint coerces to a number first, so a numeric
// string and a boolean update as numbers rather than concatenating the way the +
// operator would. In statement position the discarded result makes the prefix and
// postfix forms the same, so both read the target and assign the updated value
// back.

function bump(x: any): any {
  x++;
  return x;
}

function drop(x: any): any {
  x--;
  return x;
}

// A number updates directly.
console.log(bump(5));
console.log(drop(3));

// A numeric string and a boolean coerce to a number, not a concatenation.
console.log(bump("5"));
console.log(bump(true));

// The prefix form has the same effect in statement position.
function pre(x: any): any {
  ++x;
  return x;
}
console.log(pre(0));
