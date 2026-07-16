// A closed string-literal union is a plain string at run time, one of a fixed set
// of strings, so it lowers to value.BStr and every operation the source writes reads
// it through the ordinary string machinery: a compare against a member, a print, a
// concat, a template, a typeof, and a reassignment. This fixture drives a parameter,
// a binding, and a return each typed as such a union, so no separate tag
// representation is needed and the value flows the way JavaScript's string does.

type Dir = "north" | "south" | "east" | "west";

function opposite(d: Dir): Dir {
  if (d === "north") return "south";
  if (d === "south") return "north";
  if (d === "east") return "west";
  return "east";
}

function describe(d: Dir): string {
  return "dir:" + d;
}

let cur: Dir = "north";
// A bare read prints the string.
console.log(cur);
// A return of the union reads as the string it is.
console.log(opposite(cur));
// A concat splices it through the string path.
console.log(describe(cur));
// A template literal interpolates it.
console.log(`going ${cur}`);
// typeof a string-literal union is "string".
console.log(typeof cur);
// A reassignment to another member, then a compare against a literal.
cur = "west";
console.log(cur);
console.log(cur === "west");
console.log(cur !== "north");
