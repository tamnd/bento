// String.prototype.replace and replaceAll with a regexp and a function replacement
// call the function with each match's text and substitute the string it returns. This
// is the shape url.js's encodeQuery reaches, .replace(/[!'()*]/g, c => ...). A
// non-global regexp replaces the first match, a global one every match; the closure
// sees the matched substring alone, the single-argument replacer the runtime backs.
function bracketFirst(s: string): string {
  return s.replace(/A/, function (m: string): string { return "[" + m + "]"; });
}
function bracketAll(s: string): string {
  return s.replace(/A/g, (m: string): string => "[" + m + "]");
}
function upperWords(s: string): string {
  return s.replace(/\w+/g, (w: string): string => w.toUpperCase());
}
function escapeParens(s: string): string {
  return s.replaceAll(/[()]/g, (c: string): string => "%" + c.charCodeAt(0).toString(16));
}
console.log(bracketFirst("xAxAx"));
console.log(bracketAll("xAxAx"));
console.log(upperWords("hello world"));
console.log(escapeParens("f(a)(b)"));
