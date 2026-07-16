// A module const holds a closure that reads a module binding declared later. In
// JavaScript the later const is in scope across the whole module, so the forward
// reference is legal: the closure only reads it when it runs, by which point the
// module has finished initializing and the binding holds its value. A top-level
// function reads the closure-holding binding, which forces both bindings to package
// vars, and the closure's read of the later one resolves through that shared package
// var rather than a main local that would not be in scope yet. The closure runs
// after init, so it sees 42.
const get = () => later;
const later = 42;

function run(): number {
  return get();
}

console.log(run());
