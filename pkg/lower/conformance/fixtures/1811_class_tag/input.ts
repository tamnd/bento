// Object.prototype.toString.call(x) reads a value's internal class as an
// "[object Type]" string, the idiom test262's formatSimpleValue falls back to
// when String(x) throws. The argument is any, so the tag is decided at runtime
// from the boxed value's kind.
function tag(x: any): string {
  return Object.prototype.toString.call(x);
}

console.log(tag(3));
console.log(tag("s"));
console.log(tag(true));
console.log(tag(3.5));
console.log(tag(""));
