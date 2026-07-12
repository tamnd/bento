// A String.prototype method borrowed through .call runs on the receiver coerced to
// a string first. A number receiver stringifies to its decimal form, so slice and
// charAt read from that string, and a real string receiver passes through the same
// coercion unchanged.
const a = String.prototype.slice.call(12345, 1, 3);
console.log(a);

const b = String.prototype.charAt.call(98765, 0);
console.log(b);

const c = String.prototype.toUpperCase.call("bento");
console.log(c);

const d = String.prototype.indexOf.call("hello world", "world");
console.log(d);
