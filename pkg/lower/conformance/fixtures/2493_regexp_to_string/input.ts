// A RegExp coerces to a string through RegExp.prototype.toString, "/" + source +
// "/" + flags, the literal form the program wrote. String(re), a template
// substitution, string concatenation with +, and an explicit re.toString() all read
// the same form, and the empty pattern reports its source as "(?:)" so it round-trips
// to a legal literal. A pattern that escapes a slash keeps the backslash in its
// source, so the toString stays a pattern that would re-parse to the same regexp.
const re = /ab+c/gi;
console.log(String(re));
console.log("x=" + re);
console.log(`t:${re}`);
console.log(re.toString());
const empty = /(?:)/;
console.log(String(empty));
const escaped = /[\/]+/;
console.log(String(escaped));
