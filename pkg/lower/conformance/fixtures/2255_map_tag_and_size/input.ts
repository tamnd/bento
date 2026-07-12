// A Map's size accessor reports its current entry count, and it moves with every
// mutation path: a set that adds a new key raises it, a set that overwrites an
// existing key leaves it, a delete lowers it, and clear drops it to zero. The tag
// Object.prototype.toString.call reads off a Map is "[object Map]", the string its
// Symbol.toStringTag installs, whatever the map holds at the time.
function run(): void {
  const m = new Map<string, number>();
  console.log(m.size);

  m.set("a", 1);
  m.set("b", 2);
  console.log(m.size);

  m.set("b", 20);
  console.log(m.size);

  m.delete("a");
  console.log(m.size);

  console.log(Object.prototype.toString.call(m));

  m.clear();
  console.log(m.size);
  console.log(Object.prototype.toString.call(m));
}

run();
