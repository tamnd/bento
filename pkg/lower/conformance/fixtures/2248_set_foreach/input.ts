// Set.prototype.forEach visits members in insertion order, handing the callback each
// member. The dedup means a repeated add does not revisit, and delete then re-add
// moves a member to the end, which the traversal order reflects.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);
  s.add(3);
  s.add(2);

  let total = 0;
  s.forEach((v) => {
    total += v;
  });
  console.log(total);

  s.delete(2);
  s.add(2);
  s.forEach((v) => {
    console.log(v);
  });
}

run();
