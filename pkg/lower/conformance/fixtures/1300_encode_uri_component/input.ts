// encodeURIComponent percent-encodes every code point outside the unreserved set
// A-Za-z0-9 and -_.!~*'(), taking each other code point's UTF-8 bytes, and
// decodeURIComponent reverses it. Both take a single string, so they lower to the
// URI-codec runtime functions directly. The cases cover the unreserved passthrough,
// a space and the reserved punctuation that separate the component encoder from
// encodeURI, a multibyte code point whose UTF-8 bytes each escape, and the
// round-trip back to the original string.
function run(): void {
  console.log(encodeURIComponent("unreserved-_.!~*'()"));
  console.log(encodeURIComponent("a b/c?d=e&f"));
  console.log(encodeURIComponent("café日本"));

  console.log(decodeURIComponent("a%20b%2Fc"));
  console.log(decodeURIComponent("caf%C3%A9%E6%97%A5%E6%9C%AC"));

  const s = "name=Ada Lovelace & id=42";
  const e = encodeURIComponent(s);
  console.log(e);
  console.log(decodeURIComponent(e) === s);
}

run();
