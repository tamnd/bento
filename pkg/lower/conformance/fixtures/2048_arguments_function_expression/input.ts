// A function expression carries an arguments object of its own, so a read of
// arguments inside it materializes a store from the expression's own parameters
// in the lowered closure, not the enclosing scope's. This proves arguments works
// the same in a function-expression value as in a declared function.
const len = function (a: number, b: number): number {
  return arguments.length;
};

// the function expression's own arguments has two elements.
console.log(len(1, 2));

const first = function (a: number, b: number): unknown {
  return arguments[0];
};

// the function expression reads its own first argument, 9.
console.log(first(9, 8));
