// An array destructuring pattern with no annotation infers implicit-any elements, the
// JS-as-TS shape the checker flags "Binding element 'X' implicitly has an 'any' type"
// (7031) and the AOT front door tolerates. Its trailing rest gathers the source tail
// past the fixed slots into a boxed dynamic array through IndexRest rather than a typed
// array target the boxed source cannot fill, and the rest name is marked dynamic so its
// body reads (length and indexed positions) dispatch the boxed way.
function head([first, ...rest]) {
  return String(first) + "|" + String(rest.length) + "|" + String(rest[0]) + String(rest[1]);
}
// The declaration form off a dynamic any source binds the rest the same boxed way.
function fromAny(xs: any) {
  const [a, ...rest] = xs;
  return String(a) + "|" + String(rest.length);
}
console.log(head([10, 20, 30]));
console.log(fromAny([1, 2, 3, 4]));
