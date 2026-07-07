// The dispatch kernel of test262's assert._toString: a switch over a string
// discriminant, a ternary-typeof selector on a dynamic value, a case body that
// falls through into the next, and a narrowed read of the boxed value inside
// its typeof arm.
function pick(s: string): string {
  switch (s) {
    case "a":
      return "ay";
    case "b":
      return "bee";
    default:
      return "other";
  }
}

function tag(value: any): string {
  switch (value === null ? 'null' : typeof value) {
    case 'string':
      return 'S:' + value;
    case 'number':
      if (value === 0) { return 'zero'; }
      // falls through
    default:
      return 'other';
  }
}

function accumulate(x: number): number {
  let r = 0;
  switch (x) {
    case 1:
      r = 1;
    case 2:
      r = r + 2;
      break;
    default:
      r = 9;
  }
  return r;
}

console.log(pick("a"));
console.log(pick("z"));
console.log(tag("x"));
console.log(tag(0));
console.log(tag(3));
console.log(tag(true));
console.log(accumulate(1));
console.log(accumulate(2));
console.log(accumulate(5));
