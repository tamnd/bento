// A union of one object member beside primitive members. typeof tells the arms apart,
// the narrowed branch reads the member's own methods, and the union crosses into a
// string and a boolean the way the language says each member does.

function describe(u: string | string[]): string {
  if (typeof u === "string") {
    return "string/" + u.length;
  }
  return "array/" + u.length;
}

console.log(describe("abc"));
console.log(describe(["a", "b"]));

// Stringifying runs the member's own ToPrimitive, so an array joins with commas and
// an empty one is the empty string.
function show(u: string | string[]): string {
  return `[${u}]`;
}

const empty: string[] = [];
console.log(show("abc"), show(["a", "b"]), show(empty));

// Every object is truthy however empty it is, where the empty string is not.
function truthy(u: string | string[]): boolean {
  return u ? true : false;
}

console.log(truthy(""), truthy("a"), truthy(empty), truthy(["x"]));

// typeof over the union reports what JavaScript reports for the arm that is set.
console.log(typeof describe, typeof ("x" as string | string[]));

// An array of the union holds either member per element, and a bare [] element takes
// the array arm at the union's own element type rather than the never[] it is typed.
const pairs: (string | string[])[] = ["a", ["b", "c"], []];
for (const p of pairs) {
  console.log(typeof p, `${p}`);
}

// The shape node's own test harness opens with: a platform ternary between two array
// literals whose element is that union. Both branches reach the one Go type.
const win = false;
const pwdCommand: (string | string[])[] = win
  ? ["cmd.exe", ["/d", "/c", "cd"]]
  : ["pwd", []];
console.log(pwdCommand.length, typeof pwdCommand[0], typeof pwdCommand[1]);

// A class instance beside a primitive takes an arm too, and typeof reports "object"
// for it the way it does for any other instance.
class Point {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
}

function kind(u: string | Point): string {
  return typeof u;
}

console.log(kind("a"), kind(new Point(1)));

// A number member beside an array one narrows on typeof the same way.
function total(u: number | number[]): number {
  if (typeof u === "number") {
    return u;
  }
  let sum = 0;
  for (const n of u) {
    sum += n;
  }
  return sum;
}

console.log(total(7), total([1, 2, 3]));
