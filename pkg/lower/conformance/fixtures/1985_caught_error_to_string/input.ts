// A caught error's toString is Error.prototype.toString, the "Name: message"
// form, the same string String(err) and a template substitution produce.

try {
  throw new TypeError("boom");
} catch (e: any) {
  console.log(e.toString());
}
