// A const in a nested block that shadows an outer name and is never read still needs
// Go's declared-and-not-used blank. Bento withheld the blank whenever the name
// appeared more than once in the module, so a shadowed unused binding gobuild-failed
// with "declared and not used". The blank now keys on the binding's own symbol, so
// each unused local is blanked regardless of the name being reused elsewhere.
const z = 4;
{
  const z = 5;
}
if (true) {
  const z = 1;
  console.log(z);
}
console.log(z);
