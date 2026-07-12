// Temporal.Duration.prototype.add balances the two durations across their units, which needs
// a relativeTo reference and the calendar model this slice does not carry, so it hands back
// with a named reason rather than emit wrong Go. The same ceiling covers subtract, round,
// total, with, from over a string, and compare.
function addDurations(): void {
  const d = new Temporal.Duration(0, 0, 0, 1);
  d.add(new Temporal.Duration(0, 0, 0, 1));
}
