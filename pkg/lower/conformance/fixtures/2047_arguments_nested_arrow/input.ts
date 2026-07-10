// An arrow has no arguments of its own, so a read of arguments inside it is the
// enclosing function's arguments. The enclosing function materializes the store and
// the arrow's lowered closure captures it, so the arrow reads the same count and
// elements the enclosing function would. This proves arguments across an arrow
// boundary end to end.
function count(a: number, b: number, c: number): number {
  const get = () => arguments.length;
  return get();
}

// the arrow reads the enclosing arguments, which has three elements.
console.log(count(1, 2, 3));

function first(a: number, b: number): unknown {
  const get = () => arguments[0];
  return get();
}

// the arrow reads the enclosing first argument, 9.
console.log(first(9, 8));
