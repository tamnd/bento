// A caught error is an object, so it is always truthy: it takes the then branch
// of if (e), and !e is false. This is the boolean-position use a guard writes
// before it trusts a thrown value.

try {
  throw new Error("x");
} catch (e: any) {
  if (e) {
    console.log("truthy");
  }
  if (!e) {
    console.log("falsy");
  } else {
    console.log("not falsy");
  }
}
