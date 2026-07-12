// A WeakMap keys entries by an object held weakly, exposing only get, set, has, and
// delete: no size and no iteration, because a weakly held key set has no stable count
// or order. Reference identity decides a key, so two distinct objects are two keys
// even with the same shape, a set on an existing key overwrites its value, a delete
// reports whether the key was present, and a get on an absent key is undefined.
function run(): void {
  const a = { id: 1 };
  const b = { id: 2 };
  const m = new WeakMap<{ id: number }, string>();

  console.log(m.has(a));

  m.set(a, "first");
  m.set(b, "second");
  console.log(m.has(a));

  const va = m.get(a);
  console.log(va !== undefined ? va : "none");
  const vb = m.get(b);
  console.log(vb !== undefined ? vb : "none");

  m.set(a, "updated");
  const va2 = m.get(a);
  console.log(va2 !== undefined ? va2 : "none");

  console.log(m.delete(a));
  console.log(m.has(a));
  const va3 = m.get(a);
  console.log(va3 !== undefined ? va3 : "none");
  console.log(m.delete(a));
}

run();
