// A Set lowers to value.Set: add stores a member once, has tests membership,
// delete removes it, and size counts what is left. The repeated add of "a" is the
// dedup: a Set holds each distinct value once, so adding it twice leaves size at
// two, not three. Delete then drops "a", so the second has("a") is false and size
// falls to one.
function run(): void {
  const s = new Set<string>();
  s.add("a");
  s.add("b");
  s.add("a");
  console.log(s.size);
  console.log(s.has("a"));
  s.delete("a");
  console.log(s.has("a"));
  console.log(s.size);
}

run();
