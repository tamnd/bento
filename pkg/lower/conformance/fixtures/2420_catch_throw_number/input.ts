// A thrown number that a catch recovers binds as the number itself, so a dynamic
// read of the binding sees the value the program threw: `e === 42` holds and
// `e === 0` does not. The strict compare reads the recovered value, so the number
// round-trips from the throw through the catch. The typeof tag of a caught binding
// folds to the object model the runtime keeps a caught value in, a separate
// approximation the dynamic-catch slice owns, so this fixture stays on the value
// round-trip the throw itself guarantees.
try {
  throw 42;
} catch (e) {
  console.log(e === 42);
  console.log(e === 0);
}
