// A WeakSet collects objects held weakly, exposing only add, has, and delete: no size
// and no iteration, because a weakly held member set has no stable count or order.
// Reference identity decides a member, so two distinct objects are two members even
// with the same shape, a repeated add of a present member is a no-op, and a delete
// reports whether the member was present.
function run(): void {
  const a = { id: 1 };
  const b = { id: 2 };
  const s = new WeakSet<{ id: number }>();

  console.log(s.has(a));

  s.add(a);
  s.add(b);
  console.log(s.has(a));
  console.log(s.has(b));

  s.add(a);
  console.log(s.has(a));

  console.log(s.delete(a));
  console.log(s.has(a));
  console.log(s.delete(a));
}

run();
