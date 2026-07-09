// A call through a dynamically typed callee has no static Go func to call, so it
// dispatches at runtime through the boxed value's Call, the invoke mirror of the
// dynamic member read. A static function value flowing into an any parameter boxes
// into that callable value first: the wrapper coerces each boxed argument to the
// declared parameter type, calls the function, and boxes the result back, so a
// number result, a string result, and a side-effecting void callback all cross the
// dynamic boundary and come back with the value the language binds.

function apply(fn: any, x: number) {
  return fn(x);
}
console.log(apply((y: number) => y * 2, 21));

function run(fn: any, a: number, b: number) {
  return fn(a, b);
}
console.log(run((x: number, y: number) => x + "-" + y, 3, 4));

function each(fn: any) {
  fn(1);
  fn(2);
}
each((n: number) => {
  console.log(n);
});

const g: any = 5;
try {
  g();
} catch (e) {
  console.log((e as any).name);
}
