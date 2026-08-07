// A string held in a slot the checker types any reads its prototype methods off the
// runtime rather than through a lowered call. That is the shape most of a Node program
// takes: a parameter of a boxed object method, an element off a dynamic array, and
// nearly everything a Node API hands back. Every method here delegates to the same
// value.BStr or regexp routine the statically typed path emits, so the two agree.
//
// dyn's parameter and result are typed any, so the string it hands back has no static
// type and every call below goes through the dynamic member read.

function dyn(x: any): any {
  return x;
}

const s: any = dyn("Hello, World");

// The data properties, which the dynamic read already answered, alongside the index and
// code-point family.
console.log(s.length, s[1], s.at(-1), s.charAt(0), s.charCodeAt(1), s.codePointAt(0));

// Searching, with and without a start position.
console.log(s.indexOf("o"), s.indexOf("o", 5), s.lastIndexOf("o"), s.lastIndexOf("o", 5));
console.log(s.includes("World"), s.startsWith("He"), s.endsWith("ld"), s.startsWith("l", 2));

// Cutting. An undefined end reads as omitted, so the method applies its own default
// rather than reading NaN as a bound.
console.log(s.slice(1), s.slice(1, 4), s.slice(-5), s.slice(1, undefined), s.slice(undefined, 3));
console.log(s.substring(1, 4), s.substring(4, 1), s.substr(1, 3));

// Trimming, on a receiver with whitespace at both ends.
const p: any = dyn("  pad  ");
console.log(p.trim() + "|", p.trimStart() + "|", p.trimEnd() + "|");

// Case mapping and the two identity coercions.
console.log(s.toUpperCase(), s.toLowerCase(), s.toString(), s.valueOf());

// Building: repeat, concat, and the two pads, whose optional pad string defaults to a
// space when it is left off.
console.log(s.repeat(2), s.concat("!", "?"), s.padStart(15, "*"), s.padEnd(15) + "|");

// Normalization and the lone-surrogate pair.
console.log(s.normalize(), s.isWellFormed(), s.toWellFormed());
console.log(dyn("café").normalize("NFC").length);

// Splitting, on a string separator and on a regexp, with and without a limit.
console.log(s.split(", "), s.split(", ", 1), s.split(/,\s*/));
console.log(dyn("a,b,,c").split(","));
console.log(dyn("aXbXc").split("X", 2), dyn("abc").split(undefined), dyn("abc").split(undefined, 0));

// Replacing with a string, over a literal search and over a regexp.
console.log(s.replace("o", "0"), s.replaceAll("o", "0"));
console.log(s.replace(/o/, "0"), s.replace(/o/g, "0"), s.replaceAll(/o/g, "0"));

// Replacing with a function. The replacer has no declared arity here, so it receives the
// whole argument list the language passes: the matched text, one argument per capture
// group, the match offset, and the subject.
console.log(s.replace("o", (m: string, i: number, str: string) => m + i + str.length));
console.log(s.replaceAll("o", (m: string, i: number) => "[" + i + "]"));
console.log(s.replace(/(l+)(o)/, (m: string, p1: string, p2: string, off: number, str: string) => p1 + "-" + p2 + "-" + off + "-" + str.length));
console.log(s.replace(/(l+)(o)/g, (m: string, p1: string, p2: string, off: number) => "<" + p1 + p2 + off + ">"));

// An empty search matches between every pair of code units and at both ends.
console.log(dyn("ab").replaceAll("", "-"));

// Matching and searching. A non-regexp argument is compiled as a pattern, the step the
// language takes before it delegates.
console.log(s.match(/o/), s.match(/o/g), s.match(/z/), s.match("W"));
console.log(s.search(/W/), s.search(/z/), s.search("o"));

// A name outside the surface reads as undefined the way a miss on any other receiver
// does, rather than answering something that would give a wrong result.
console.log(dyn("x").nope === undefined);
