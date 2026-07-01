package runtime

import (
	"strings"
	"testing"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

func TestURLParsesComponents(t *testing.T) {
	script := `
		const u = new URL("https://user:pass@example.com:8443/a/b?x=1&y=2#frag");
		console.log([
			u.protocol, u.username, u.password, u.hostname, u.port,
			u.pathname, u.search, u.hash, u.origin,
		].join("|"));
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "https:|user|pass|example.com|8443|/a/b|?x=1&y=2|#frag|https://example.com:8443"
	if out != want {
		t.Errorf("url components = %q, want %q", out, want)
	}
}

func TestURLResolvesAgainstBase(t *testing.T) {
	script := `
		const u = new URL("../c/d?q=1", "https://example.com/a/b/page");
		console.log(u.href);
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "https://example.com/a/c/d?q=1" {
		t.Errorf("resolved href = %q, want %q", out, "https://example.com/a/c/d?q=1")
	}
}

func TestURLInvalidThrows(t *testing.T) {
	script := `
		try {
			new URL("not a url");
			console.log("no throw");
		} catch (e) {
			console.log(e instanceof TypeError ? "TypeError" : "other");
		}
		console.log(String(URL.canParse("https://ok.test/")));
		console.log(String(URL.canParse("nope")));
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "TypeError\ntrue\nfalse" {
		t.Errorf("invalid url handling = %q", out)
	}
}

func TestURLSearchParamsLiveView(t *testing.T) {
	script := `
		const u = new URL("https://example.com/?a=1&b=2");
		u.searchParams.append("a", "3");
		u.searchParams.set("b", "9");
		console.log(u.searchParams.getAll("a").join(","));
		console.log(u.searchParams.get("b"));
		console.log(u.search);
		console.log(u.href);
	`
	out := strings.TrimSpace(runToEnd(t, script))
	// set replaces the first match in place and drops the rest, so b stays
	// between the two a values, matching Node and the browser.
	want := "1,3\n9\n?a=1&b=9&a=3\nhttps://example.com/?a=1&b=9&a=3"
	if out != want {
		t.Errorf("searchParams live view = %q, want %q", out, want)
	}
}

func TestURLSearchParamsStandalone(t *testing.T) {
	script := `
		const p = new URLSearchParams("q=hello world&tag=a&tag=b");
		console.log(p.get("q"));
		console.log(p.getAll("tag").join(","));
		p.delete("tag");
		console.log(p.toString());
		const fromObj = new URLSearchParams({ name: "duc", city: "hcm" });
		console.log(fromObj.toString());
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "hello world\na,b\nq=hello+world\nname=duc&city=hcm"
	if out != want {
		t.Errorf("standalone searchParams = %q, want %q", out, want)
	}
}

func TestURLSetters(t *testing.T) {
	script := `
		const u = new URL("http://example.com/path");
		u.protocol = "https:";
		u.hostname = "other.test";
		u.port = "9000";
		u.pathname = "/new";
		u.hash = "top";
		console.log(u.href);
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "https://other.test:9000/new#top" {
		t.Errorf("setter href = %q, want %q", out, "https://other.test:9000/new#top")
	}
}

func TestTextEncoderDecoderRoundTrip(t *testing.T) {
	script := `
		const enc = new TextEncoder();
		const bytes = enc.encode("héllo");
		console.log(enc.encoding + " " + bytes.length + " " + (bytes instanceof Uint8Array));
		const dec = new TextDecoder();
		console.log(dec.decode(bytes));
		const utf16 = new TextDecoder("utf-16le");
		console.log(utf16.decode(Buffer.from("hi", "utf16le")));
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "utf-8 6 true\nhéllo\nhi"
	if out != want {
		t.Errorf("text encoding round trip = %q, want %q", out, want)
	}
}

func TestURLModuleRequire(t *testing.T) {
	script := `
		const { URL, URLSearchParams } = require("url");
		console.log(typeof URL);
		console.log(typeof URLSearchParams);
		const u = new URL("https://a.test/x");
		console.log(u.hostname);
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "function\nfunction\na.test" {
		t.Errorf("url module require = %q", out)
	}
}
