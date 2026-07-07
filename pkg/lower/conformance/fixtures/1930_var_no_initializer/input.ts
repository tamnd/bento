// A binding declared with no initializer holds undefined until its first
// assignment, the way `var x;` reads undefined in JavaScript. A dynamic binding
// (any or unknown) lowers to value.Value, whose Go zero value is exactly that
// undefined, so a bare declaration with no value reads undefined before it is
// set and the assigned value after. This is the shape assert.throws opens with,
// two names declared together and assigned later inside the try.
function check(): void {
  let expectedName: any, actualName: any;
  console.log(expectedName === undefined);
  console.log(actualName === undefined);
  expectedName = "TypeError";
  actualName = "RangeError";
  console.log(expectedName);
  console.log(actualName);
}
check();
