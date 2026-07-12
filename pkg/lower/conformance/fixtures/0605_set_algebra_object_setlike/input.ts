// A set-algebra method accepts any set-like: an object with a size, a has, and a keys
// iterator. The two built-in set-likes, a Set and a Map, lower to concrete runtime
// types the algebra reads directly. A set-like a program builds by hand as a plain
// object literal, though, carries its has and keys as method members, which the object
// path does not lower, so the whole unit hands back rather than mislower the set-like
// argument. The dynamic hand-built set-like is a later slice.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);

  const setLike: ReadonlySetLike<number> = {
    has(v: number): boolean {
      return v === 2;
    },
    keys(): IterableIterator<number> {
      return [2].values();
    },
    size: 1,
  };

  const u = s.union(setLike);
  console.log(u.size);
}

run();
