// URLSearchParams on its own, away from any URL: the multimap reads, the mutations,
// and the form-urlencoded serialization. The query repeats a name, which is what
// getAll exists to read and what set is defined to collapse.
const p = new URLSearchParams("a=1&b=2&a=3&flag");
console.log(String(p.size));
console.log(String(p.has("a")));
console.log(String(p.has("missing")));
console.log(p.getAll("a").join(","));
console.log(p.toString());

p.append("c", "4");
console.log(p.toString());

// set replaces the first pair with the name and drops the rest, keeping its position.
p.set("a", "9");
console.log(p.toString());

p.delete("b");
console.log(p.toString());

p.sort();
console.log(p.toString());

p.forEach((value, name) => {
  console.log(name + "->" + value);
});

// The serializer writes a space as "+" and percent encodes everything outside the
// unreserved set, including the characters encodeURIComponent itself spares.
const enc = new URLSearchParams();
enc.append("a b", "c&d=e");
enc.append("greek", "π");
enc.append("punct", "*!'()~");
console.log(enc.toString());

// And the serialization parses back to the pairs it came from.
const back = new URLSearchParams(enc.toString());
console.log(back.getAll("a b").join(","));
console.log(back.getAll("punct").join(","));
console.log(String(back.size));
