// An Error subclass whose super() argument is dynamic rather than statically a
// string: options.message is read off an any-typed parameter, so `options.message ||
// fallback` types any and the inherited message field cannot take it raw. The
// argument boxes and coerces through value.ErrorMessageString, the constructor's rule
// that undefined leaves the message empty and any present value takes ToString. It is
// the shape node:assert's AssertionError hands super().
class AssertError extends Error {
  constructor(o: any) {
    super(o.message || "Assertion failed");
    this.name = "AssertError";
  }
}
const a = new AssertError({ message: "values differ" });
console.log(a.message);
console.log(a.name);
const b = new AssertError({});
console.log(b.message);
console.log(String(b instanceof Error));
