// Audit wave W2: a `key in obj` test whose key names an Object.prototype member folds to
// the constant true on a static object shape, since that member lives on the prototype
// chain of every ordinary object no matter what its own fields declare. A required own
// property folds true the same way, while a name the shape declares under neither an own
// field nor a prototype member keeps its honest handback rather than fold to a false an
// index signature could contradict.

function probe(): string {
  const o = { a: 1, b: 2 };
  const parts: string[] = [];
  // A required own property is always present.
  parts.push("a=" + ("a" in o));
  // toString and valueOf live on Object.prototype, so they hold though the shape
  // declares no such own field.
  parts.push("toString=" + ("toString" in o));
  parts.push("valueOf=" + ("valueOf" in o));
  parts.push("hasOwnProperty=" + ("hasOwnProperty" in o));
  parts.push("constructor=" + ("constructor" in o));
  return parts.join(" ");
}

console.log(probe());

// An optional own field whose name is also a prototype member still holds: the prototype
// member stands in whenever the own field is absent.
function optionalName(o: { toString?: () => string }): boolean {
  return "toString" in o;
}

console.log(optionalName({}));
