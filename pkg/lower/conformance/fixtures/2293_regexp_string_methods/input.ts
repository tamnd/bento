// The String methods that take a regexp delegate to the RegExp engine: search
// reports the first match's index or -1, match returns the match array (every matched
// substring under the global flag), replace and replaceAll substitute a template that
// expands $&, $`, and $n, and split cuts the subject at each separator and keeps the
// separator's capture groups between the pieces. A non-global match and a split both
// return an array the program then reads by index and length.
console.log("aabbbc".search(/b+/));
console.log("xyz".search(/b+/));

const m = "xabbby".match(/a(b+)/);
if (m !== null) {
  console.log(m[0]);
  console.log(m[1]);
}

const all = "axaybz".match(/a./g);
if (all !== null) {
  console.log(all.length);
  console.log(all[0]);
  console.log(all[1]);
}

console.log("banana".replace(/a/, "X"));
console.log("ab".replace(/(a)(b)/, "$2$1"));
console.log("abbbc".replace(/b+/, "[$&]"));
console.log("a1b2".replaceAll(/([a-z])(\d)/g, "$2$1"));

const parts = "a,b,c".split(/,/);
console.log(parts.length);
console.log(parts[0]);
console.log(parts[2]);

const caps = "a1b2c".split(/(\d)/);
console.log(caps.length);
console.log(caps[1]);
console.log(caps[3]);

const limited = "a,b,c,d".split(/,/, 2);
console.log(limited.length);
console.log(limited[1]);
