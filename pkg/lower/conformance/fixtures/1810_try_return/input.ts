// Returns escaping a try, the shape assert.throws and every try-probing
// test262 helper is built from: a try and catch that both return, a catch
// return past a void try body with a finally still running, and a finally
// return overriding the try's.
function safeInvert(x: number): number {
  try {
    if (x === 0) {
      throw new Error("div by zero");
    }
    return 1 / x;
  } catch (e) {
    return -1;
  }
}

function guard(x: number): string {
  try {
    if (x < 0) {
      throw new Error("neg");
    }
  } catch (e) {
    return "caught";
  } finally {
    console.log("fin");
  }
  return "ok";
}

function overridden(): number {
  try {
    return 1;
  } finally {
    return 2;
  }
}

console.log(safeInvert(4));
console.log(safeInvert(0));
console.log(guard(1));
console.log(guard(-1));
console.log(overridden());
