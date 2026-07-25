// url.searchParams is a live view: a mutation through it re-serializes the owning
// URL's search and href, and reading the view twice gives the same object. A view
// built by copy is not owned by any URL, so mutating it leaves the source alone.
const u = new URL("https://example.com/p?a=1#f");
const params = u.searchParams;

params.append("b", "two words");
console.log(u.search);
console.log(u.href);

params.set("a", "9");
console.log(u.search);
console.log(u.href);

params.delete("a");
params.delete("b");
console.log(u.search);
console.log(u.href);

// The view is the same object every read, so a mutation through one reading is seen
// through the other.
u.searchParams.append("z", "1");
console.log(String(params.size));
console.log(u.search);

const copy = new URLSearchParams(u.searchParams);
copy.append("only-on-the-copy", "1");
console.log(copy.toString());
console.log(u.search);
