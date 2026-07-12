// A named capture group (?<name>...) exposes its match on the result's groups object,
// keyed by the name, and $<name> in a replacement substitutes the named group's text.
// The pattern lowers because RE2 hosts a named group as (?P<name>...), its own spelling
// of the same construct, so the group's text flows through exec and replace unchanged.
const m = /(?<year>\d{4})-(?<month>\d{2})/.exec("2026-07");
if (m !== null && m.groups !== undefined) {
  console.log(m.groups.year);
  console.log(m.groups.month);
}

const d = "2026-07-12".match(/(?<y>\d{4})-(?<mo>\d{2})/);
if (d !== null && d.groups !== undefined) {
  console.log(d.groups.y);
  console.log(d.groups.mo);
}

console.log("2026-07".replace(/(?<y>\d{4})-(?<mo>\d{2})/, "$<mo>/$<y>"));
console.log("abc".replace(/(?<x>b)/, "[$<x>]"));
