// The WHATWG URL: construction from an absolute input, construction against a base,
// and every component the specification exposes as an accessor. The input carries a
// userinfo pair, a non-default port, a query and a fragment, so each getter has
// something to report rather than the empty string. canParse asks the same question
// the constructor answers without paying for the exception.
const u = new URL("https://user:pw@example.com:8443/a/b?x=1&y=2#frag");
console.log(u.href);
console.log(u.protocol);
console.log(u.username);
console.log(u.password);
console.log(u.host);
console.log(u.hostname);
console.log(u.port);
console.log(u.pathname);
console.log(u.search);
console.log(u.hash);
console.log(u.origin);
console.log(u.toString());
console.log(u.toJSON());

const rel = new URL("../c?q=1", "https://example.com/a/b/d");
console.log(rel.href);
console.log(rel.pathname);

// A URL with no path reports "/", and a default port reports the empty string.
const bare = new URL("http://example.com");
console.log(bare.pathname);
console.log(bare.port);

console.log(String(URL.canParse("https://example.com")));
console.log(String(URL.canParse("/relative")));
console.log(String(URL.canParse("/relative", "https://example.com")));
