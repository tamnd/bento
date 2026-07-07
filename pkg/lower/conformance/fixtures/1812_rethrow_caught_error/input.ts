// Re-throwing a caught error is the rethrow half of the exception model: a catch
// that cannot handle what it caught passes the value straight back up. The
// binding is already the runtime's error, so throw err re-raises that exact
// error rather than wrapping it again, and an outer catch reads the same name and
// message the inner throw set. test262's assert prelude leans on this in
// formatSimpleValue, which rethrows anything String(value) throws that is not a
// TypeError.
function rethrow(): void {
  try {
    throw new TypeError("boom");
  } catch (err: any) {
    throw err;
  }
}

try {
  rethrow();
} catch (e: any) {
  console.log(e.name);
  console.log(e.message);
}
