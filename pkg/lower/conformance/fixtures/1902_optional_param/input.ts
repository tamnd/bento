// A dynamic optional parameter is the shape the test262 assert prelude leans on:
// every member reads a trailing message?: any and works whether the caller passes
// it or not. The callback type here carries that same trailing optional, so the
// lowered Go func type must match the arrow assigned to it, and a call that omits
// the argument fills the slot with the undefined the language binds there.
function run(cb: (a: number, b?: any) => void): void {
  cb(1);
  cb(2, "given");
}

run((a: number, b?: any): void => {
  console.log(a);
  console.log(b);
});
