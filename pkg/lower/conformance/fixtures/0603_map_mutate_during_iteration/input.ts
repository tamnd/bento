// A for...of over a Map lowers to a range over an insertion-ordered snapshot taken
// once at loop entry, which is faithful only when the body reads the Map. A body that
// deletes an entry not yet reached, or adds one ahead of the cursor, would in
// JavaScript see the iterator's live view of that change, which the snapshot cannot
// show, so a loop that mutates the very Map it iterates hands back rather than emit a
// range that silently diverges. The live iterator is a later slice.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);
  for (const k of m.keys()) {
    if (k === "a") {
      m.delete("c");
    }
  }
  console.log(m.size);
}

run();
