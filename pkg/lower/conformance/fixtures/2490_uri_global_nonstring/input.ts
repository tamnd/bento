// The URI and base64 global codecs run ToString on their argument before their own
// work, so a non-string argument is coerced rather than handed back. This is the
// shape url.js's encodeQuery(str) reaches, an any parameter handed straight to
// encodeURIComponent. A string argument keeps the direct path; a number, a boolean,
// or an any binding is stringified the way the global does, then encoded.
function enc(x: any): string {
  return encodeURIComponent(x);
}
console.log(enc("a b&c=d"));
console.log(enc(42));
console.log(enc(true));
console.log(encodeURI("http://a.b/x y?q=1"));
console.log(btoa(String(123)));
