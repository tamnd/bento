// A named function expression names itself, and that name is in scope only inside
// its own body, where a recursive call reads it. Go has no self-referential
// function literal, so the closure binds to a declared local first and the literal
// is assigned second, which lets the body call the local by name. A named function
// expression whose body never reads its name needs no such step and lowers as a
// plain closure.

const fac = function f(n: number): number {
  return n <= 1 ? 1 : n * f(n - 1);
};

const fib = function self(n: number): number {
  return n < 2 ? n : self(n - 1) + self(n - 2);
};

const inc = function named(n: number): number {
  return n + 1;
};

console.log(fac(5));
console.log(fib(10));
console.log(inc(41));
