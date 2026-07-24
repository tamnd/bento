// The url module resolves a URL through the __bento_url_parse host callee, which
// the AOT path routes to nodehost.URLParseJSON (Go's net/url behind it) and reads
// back as the WHATWG component set. This fixture parses an absolute URL with a
// userinfo, an explicit port, a query, and a fragment, then a relative reference
// resolved against a base, and prints the components a browser URL exposes. The
// oracle is the same output Node's URL produces, so it pins the bridge against the
// reference the module targets.
const abs = JSON.parse(__bento_url_parse("http://user@host.com:8080/a/b?x=1#h", ""));
console.log(abs.protocol, abs.hostname, abs.port, abs.pathname, abs.search, abs.hash, abs.origin, abs.username);

const rel = JSON.parse(__bento_url_parse("../c?y=2", "http://host.com/a/b/"));
console.log(rel.href, rel.pathname, rel.search, rel.origin);

const bad = JSON.parse(__bento_url_parse("not a url", ""));
console.log(bad.ok);
