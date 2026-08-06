// A closure cannot fill a defaulted parameter at its call sites the way a top-level
// function does, since the call site is not always in reach: an arrow read as a value, a
// function reached through an object, an export a require pulls in. So the callee fills
// the default in its own body instead, and the parameter takes the boxed slot the
// undefined an omitted argument binds needs.

// An arrow reached through an object literal. The object's field type has to say the same
// thing the arrow's own declaration does, or the two would not fit.
const greet = (a: number, b = "d") => String(a) + "|" + b;
const box = { greet };
console.log(box.greet(1), box.greet(2, "z"), greet(3, undefined));

// A default that reads an earlier parameter is evaluated in the callee's scope, where
// that parameter is bound, so no call site could reconstruct it.
const pair = function (a: number, b = a + 1) {
  return a + b;
};
const call = pair;
console.log(call(2), call(2, 10));

// A function declared inside another function is a closure too, however it is reached.
function outer(): string {
  function label(x: number, y = 5): number {
    return x * y;
  }
  const f = label;
  return String(label(3)) + "|" + String(f(3, 2));
}
console.log(outer());

// An explicit undefined takes the default the same way an omission does, which is the
// rule that makes the body-entry fill the right spelling.
const tail = (a: number, b = 7, c = b * 2) => a + b + c;
console.log(tail(1), tail(1, 2), tail(1, undefined, 3));

// An arrow whose every use is a direct call still fills at the call site, which keeps its
// parameter's static Go type. Both lowerings are live and have to agree on the answer.
const direct = (x = 5) => x * 2;
console.log(direct(), direct(4));
