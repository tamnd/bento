// A thrown object binds as the object itself, not the {name, message} shape the
// runtime models an error with, so the catch sees an object value: `typeof e` is
// "object" and the binding is not null. Reading a specific own property back off the
// binding is a separate caught-binding slice; this proves the throw round-trips the
// object into the catch as an object.
try {
  throw { code: 7 };
} catch (e) {
  console.log(typeof e);
  console.log(e !== null);
}
