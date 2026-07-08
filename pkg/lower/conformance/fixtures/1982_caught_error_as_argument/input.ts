// A caught error passed to a helper that takes any boxes to the dynamic object
// the error presents, so the helper reads its name and message through the same
// dynamic property path any other object takes. This is the shape assert.throws
// uses when it hands a thrown value to a message-building helper.

function describe(x: any): string {
  return "got " + x.name + ": " + x.message;
}

try {
  throw new TypeError("bad input");
} catch (e: any) {
  console.log(describe(e));
}
