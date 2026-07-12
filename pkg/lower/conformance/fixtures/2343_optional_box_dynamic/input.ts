// A T | undefined result read into a dynamic sink, console.log's any argument,
// boxes through the value model: a present element renders the way String does and
// a missing one renders as "undefined". Array.prototype.at gives a numeric optional
// and String.prototype.at a string one, the two element boxes the coercion spells.
const a: number[] = [10, 20, 30];
console.log(a.at(0));
console.log(a.at(-1));
console.log(a.at(10));

const s = "hi";
console.log(s.at(0));
console.log(s.at(5));
