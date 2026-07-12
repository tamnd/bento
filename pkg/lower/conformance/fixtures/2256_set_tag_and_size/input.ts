// A Set's size accessor reports its current member count, and it moves with every
// mutation path: an add of a new member raises it, an add of a member already
// present leaves it, a delete lowers it, and clear drops it to zero. The tag
// Object.prototype.toString.call reads off a Set is "[object Set]", the string its
// Symbol.toStringTag installs, whatever the set holds at the time.
function run(): void {
  const s = new Set<number>();
  console.log(s.size);

  s.add(1);
  s.add(2);
  console.log(s.size);

  s.add(2);
  console.log(s.size);

  s.delete(1);
  console.log(s.size);

  console.log(Object.prototype.toString.call(s));

  s.clear();
  console.log(s.size);
  console.log(Object.prototype.toString.call(s));
}

run();
