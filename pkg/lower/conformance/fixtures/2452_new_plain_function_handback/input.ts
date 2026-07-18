// A new over a value the checker types as a plain function draws the checker's
// construct-signature report (7009), a strictness artifact over JavaScript that
// builds a fresh object with the callable as its constructor at run time. The front
// door tolerates the report so the program reaches the renderer, but the renderer
// lowers a new only for a class or a named built-in constructor, so a plain-function
// target hands back to the engine with the generic constructor reason rather than
// emitting a wrong construction.
function Widget(): void {}

const w = new Widget();
console.log(typeof w);
