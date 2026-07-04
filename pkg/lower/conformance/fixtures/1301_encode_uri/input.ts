// encodeURI leaves the unreserved set A-Za-z0-9 -_.!~*'() and the reserved
// delimiters ; / ? : @ & = + $ , # alone and percent-encodes every other code
// point's UTF-8 bytes, so it keeps a whole URI's structure where
// encodeURIComponent would escape the delimiters. decodeURI reverses it but leaves
// an escape that names a reserved delimiter as its literal %XX, so the decoded URI
// keeps the escaped punctuation a re-encode would restore anyway. The cases cover
// a whole URI with a space and a multibyte code point, the reserved set passing
// through untouched, and the preserve rule that separates decodeURI from
// decodeURIComponent.
function run(): void {
  const u = "http://a.b/c d?x=café&y=z#f";
  const e = encodeURI(u);
  console.log(e);
  console.log(decodeURI(e));
  console.log(decodeURI(e) === u);

  console.log(encodeURI(";,/?:@&=+$#"));
  console.log(decodeURI("%3Bx%2Fy"));
}

run();
