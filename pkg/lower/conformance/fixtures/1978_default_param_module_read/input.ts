// A default parameter lets a caller omit the argument, and Go has no optional
// argument, so the omitting call site fills the slot with the default expression.
// A default that reads a module binding lowers there directly, since the binding
// is hoisted to a package var and stays visible at the call site; a default that
// calls a top-level function lowers the same way, since the function is always
// package-visible. Each function below is called once with the argument omitted,
// so the default fills the slot, and once with the argument supplied, so it wins.

const base = 10;

function factor(): number {
  return 3;
}

function offset(n: number = base): number {
  return n;
}

function scale(n: number = factor()): number {
  return n * 2;
}

console.log(offset());
console.log(offset(4));
console.log(scale());
console.log(scale(5));
