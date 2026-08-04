// A plain function that reads this with no this annotation draws the checker's
// implicit-this report (2683), a strictness artifact over JavaScript that binds this
// at the call site. The front door tolerates the report so the program reaches the
// renderer, and the renderer answers what the call supplies: nothing, which is
// undefined once the code is strict. This unit is not strict, it carries no "use
// strict" prologue and is a script rather than a module, so this is the global object
// instead, a value bento has none of, and the unit hands back to the engine with its
// own named reason rather than emitting a wrong reference.
function describe(): string {
  return typeof this;
}

console.log(describe());
