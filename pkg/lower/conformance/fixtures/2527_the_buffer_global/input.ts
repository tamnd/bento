// Buffer is Node's byte array, and a Node program reaches for it constantly: every read
// off a socket or a file arrives as one. It is a Uint8Array with a wider prototype, the
// same bytes over the same buffer, so everything the typed-array family already does
// works over a Buffer unchanged and the members below are the ones Buffer adds.
//
// Nothing here lowers statically. Buffer is an ambient any, so the name lowers to the
// runtime's one constructor and every member read and call off it dispatches
// dynamically, which is what this fixture is really exercising: the whole surface
// reached the way a compiled program reaches it.

// The three allocators. allocUnsafe is alloc here, because Go's allocator hands back
// zeroed pages and there is no way to ask it not to.
const a = Buffer.alloc(4);
console.log(a.length, a.toString("hex"));
console.log(Buffer.alloc(5, "ab").toString(), Buffer.allocUnsafe(3).length);

// from takes a string in any of the encodings, and toString reads one back. The base64
// and hex round trips are what a program parsing a wire format does all day.
const hello = Buffer.from("hello world");
console.log(hello.toString("hex"));
console.log(hello.toString("base64"), hello.toString("base64url"));
console.log(Buffer.from("aGVsbG8=", "base64").toString());
console.log(Buffer.from("68656c6c6f", "hex").toString());
console.log(hello.toString("utf8", 0, 5), hello.toString(undefined, 6));

// A non-ASCII code point is several bytes in utf8 and one in latin1, so the two
// encodings disagree about how long the same string is.
console.log(Buffer.byteLength("héllo"), Buffer.byteLength("héllo", "latin1"));
console.log(Buffer.from("héllo", "latin1").toString("hex"));

// A write with no room for the last code point stops on a boundary rather than leaving
// a fragment that would read back as a replacement character.
const room = Buffer.alloc(4);
console.log(room.write("€uro"), room.toString("hex"));

// slice and subarray are the same function on a Buffer, and both are windows onto the
// receiver's bytes rather than copies. This is where Buffer diverges from Array, and a
// program that assumed otherwise corrupts its own data.
const head = hello.slice(0, 5);
head[0] = 0x48;
console.log(hello.toString(), head.toString(), hello.subarray(-5).toString());

// The searches take a string, a byte, or another buffer.
console.log(hello.indexOf("world"), hello.indexOf(0x6f), hello.lastIndexOf("o"));
console.log(hello.includes(Buffer.from("llo")), hello.indexOf("zz"));

// fill repeats its pattern across the window rather than writing it once.
const filled = Buffer.alloc(6);
filled.fill("xy", 1, 5);
console.log(filled.toString("hex"));

// copy is a memmove, so an overlapping copy inside one buffer moves the bytes rather
// than smearing the first one across the range.
const over = Buffer.from("abcdef");
over.copy(over, 2, 0, 4);
console.log(over.toString());

// The numeric family reads and writes fixed-width integers and floats at an offset, in
// either byte order, which is the whole reason a program parsing a binary format wants
// a Buffer rather than an array.
const n = Buffer.alloc(8);
n.writeUInt32BE(0xdeadbeef, 0);
console.log(n.toString("hex", 0, 4), n.readUInt32BE(0), n.readInt32BE(0));
n.writeUInt32LE(0xdeadbeef, 0);
console.log(n.toString("hex", 0, 4), n.readUint32LE(0));
n.writeInt16LE(-2, 0);
console.log(n.readInt16LE(0), n.readUInt16LE(0), n.readInt16BE(0));
n.writeDoubleBE(-0.5, 0);
console.log(n.toString("hex"), n.readDoubleBE(0));

// The variable-width members take a byte count instead of carrying one in their name,
// so a program can read the three- and six-byte fields no other API offers.
const v = Buffer.alloc(6);
v.writeUIntBE(0x010203040506, 0, 6);
console.log(v.toString("hex"), v.readUIntBE(0, 6), v.readUIntLE(0, 6));

// A read past the end throws rather than answering undefined, because a read past the
// end of a buffer is the mistake a parser makes on malformed input.
try {
  n.readUInt32BE(6);
} catch (e) {
  console.log((e as any).code, (e as any).message);
}
try {
  Buffer.from(5 as any);
} catch (e) {
  console.log((e as any).code);
}

// The comparisons and the concatenation.
console.log(hello.equals(Buffer.from("Hello world")), Buffer.compare(Buffer.from("b"), Buffer.from("a")));
console.log(Buffer.concat([Buffer.from("ab"), Buffer.from("cd")]).toString());
console.log(Buffer.concat([Buffer.from("ab"), Buffer.from("cd")], 3).toString());

// The three questions a program asks about a value's kind do not all answer the same
// way: a Buffer's constructor is Buffer and it passes instanceof Buffer, and yet
// Object.prototype.toString calls it a Uint8Array, since that tag comes from the
// typed-array prototype and Buffer.prototype adds none of its own.
console.log(hello instanceof Buffer, Buffer.isBuffer(hello), Buffer.isBuffer("hello"));
console.log(hello.constructor.name, Object.prototype.toString.call(hello));
console.log(Buffer.isEncoding("UTF-8"), Buffer.isEncoding("nope"));

// A Buffer carries a toJSON hook, so unlike an ArrayBuffer it survives a round trip
// through JSON.
const small = Buffer.from("abc");
console.log(JSON.stringify(small), Buffer.from(small.toJSON().data).toString());

// The typed-array half is untouched: the indices, the geometry, and the family's own
// members all still work over the same bytes.
console.log(small[0], small.length, small.byteLength, small.BYTES_PER_ELEMENT);
// .buffer is the storage under the view, and its length is not pinned here: node carves
// a small buffer out of a shared eight-kilobyte pool, so its .buffer is the pool and its
// byteOffset is wherever in the pool it landed. bento gives every buffer its own storage.
// No correct program depends on either answer.
console.log(small.at(-1), small.includes(0x62), small.buffer.byteLength >= small.length);

// And a Buffer prints as its bytes rather than as the element list its Uint8Array half
// would take.
console.log(small);
console.log(Buffer.alloc(0));

// The module and the global are one constructor, which is a thing Node's own test
// helpers check directly.
const mod = require("buffer");
console.log(mod.Buffer === Buffer, mod.kMaxLength, mod.constants.MAX_STRING_LENGTH);
