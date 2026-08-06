// A new over a value the checker types as a plain function draws the checker's
// construct-signature report (7009), a strictness artifact over JavaScript that
// builds a fresh object with the callable as its constructor at run time. The front
// door tolerates the report so the program reaches the renderer, and the renderer
// lowers the function to a runtime constructor value, so the new builds a real
// object linked to the function's prototype rather than handing back to the engine.
function Widget(): void {}

const w = new Widget();
console.log(typeof w);
console.log(Object.getPrototypeOf(w) === Widget.prototype);
