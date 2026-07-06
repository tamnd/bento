// The mangle is a pure function of the name, so $DONE always spells D_DONE
// in the emitted Go. A module that also declares D_DONE verbatim would clash
// in the output, and renaming either side would make emission depend on what
// else the module declares, so the whole module hands back instead.
function $DONE(): void {
  console.log("mangled");
}

function D_DONE(): void {
  console.log("verbatim");
}

$DONE();
D_DONE();
