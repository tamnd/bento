// A Set's iteration order follows the same insertion-order guarantee a Map's does.
// Adding a member already present leaves it in place, deleting a member removes it
// without disturbing the rest, and deleting then re-adding a member gives it a new
// position at the end. The order a for...of observes after a run of such mutations is
// exactly the surviving members in first-insertion order, each re-addition at the tail.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);
  s.add(3);
  s.add(4);
  s.add(5);

  s.delete(2);
  s.delete(4);
  s.add(1); // already present, keeps its position
  s.add(2); // re-added, appends at the end

  let order = "";
  for (const v of s) {
    order += v;
  }
  console.log(order);
  console.log(s.size);
}

run();
