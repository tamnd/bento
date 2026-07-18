// A plain function that reads this with no this annotation draws the checker's
// implicit-this report (2683), a strictness artifact over JavaScript that binds this
// at the call site. The front door tolerates the report so the program reaches the
// renderer, but the renderer lowers this only inside a class body it is currently
// lowering, so a free this finds no receiver and hands back to the engine with its
// own named reason rather than emitting a wrong reference.
function describe(): string {
  return typeof this;
}

console.log(describe());
