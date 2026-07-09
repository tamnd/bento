// A binding or property named _ is an ordinary readable name in JavaScript, but _
// is Go's blank identifier, which discards its value and cannot be read. Emitting
// it verbatim would leave `_ := 1` unreadable and refuse to compile the moment the
// name is used, so the lowerer escapes the lone underscore to a readable spelling
// that a declaration and every reference agree on. The test262 identifiers suite
// exercises this directly with `var _ = 1`.
const _ = 1;
const o = { _: 2 };
function add(_: number): number {
  return _ + o._;
}
console.log(_ + add(3));
