// A RegExp flowing into an any binding boxes into a dynamic value: typeof reports
// "object", the box stringifies to its literal form the way String and a template
// substitution render a concrete regexp, its own accessors (.source, .flags, the flag
// booleans, .lastIndex) read off the live regexp, the box is truthy, and it compares
// equal to itself by identity. A dynamic call of .test or .exec off the box is a later
// slice, so this fixture reads the box rather than matching with it.
const r: any = /ab+c/gi;
console.log(typeof r);
console.log(String(r));
console.log("x=" + r);
console.log(`t:${r}`);
console.log(r.source);
console.log(r.flags);
console.log(r.global);
console.log(r.sticky);
console.log(r.lastIndex);
console.log(r ? "truthy" : "falsy");
console.log(r === r);
