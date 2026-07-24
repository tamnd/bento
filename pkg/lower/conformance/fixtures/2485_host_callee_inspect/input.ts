// util.inspect and the assert message formatter render a value through the
// __bento_inspect host callee, which the AOT path resolves to value.Inspect: the
// compact object-and-array spelling the interpreter's prelude inspector shares.
// This fixture stays to the shapes where that spelling matches Node's util.inspect
// exactly (numbers, nested objects and arrays, the empty forms), so the oracle is
// real Node output. Strings are left out on purpose: the prelude inspector renders
// a top-level string unquoted and a nested one JSON-double-quoted, where Node uses
// single quotes, a fidelity gap the shared inspector carries and a later slice can
// close by teaching both paths Node's string quoting.
console.log(__bento_inspect(42));
console.log(__bento_inspect(true));
console.log(__bento_inspect(null));
console.log(__bento_inspect([1, 2, 3]));
console.log(__bento_inspect({ a: 1, b: { c: 2 } }));
console.log(__bento_inspect([]));
console.log(__bento_inspect({}));
