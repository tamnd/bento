package runtime

import (
	"strings"
	"testing"
)

// TestDNSLookupLocalhost resolves localhost through dns.lookup, which is OS name
// resolution and reads the hosts file, so it settles without a network DNS query.
func TestDNSLookupLocalhost(t *testing.T) {
	out := runToEnd(t, `
		const dns = require("node:dns");
		dns.lookup("localhost", (err, address, family) => {
			if (err) throw err;
			const ok = address === "127.0.0.1" || address === "::1";
			console.log(ok ? "ok" : "bad:" + address, family);
		});
	`)
	if !strings.Contains(out, "ok") {
		t.Fatalf("lookup localhost failed: %q", out)
	}
	if !strings.Contains(out, "4") && !strings.Contains(out, "6") {
		t.Fatalf("missing family: %q", out)
	}
}

// TestDNSLookupAll exercises the all:true path, which returns an array of
// {address, family} instead of a single address.
func TestDNSLookupAll(t *testing.T) {
	out := runToEnd(t, `
		const dns = require("node:dns");
		dns.lookup("localhost", { all: true }, (err, addresses) => {
			if (err) throw err;
			console.log(Array.isArray(addresses), addresses.length > 0, typeof addresses[0].family);
		});
	`)
	if !strings.Contains(out, "true true number") {
		t.Fatalf("lookup all failed: %q", out)
	}
}

// TestDNSPromisesLookup covers the dns.promises face over the same core.
func TestDNSPromisesLookup(t *testing.T) {
	out := runToEnd(t, `
		const dns = require("node:dns");
		dns.promises.lookup("localhost").then((res) => {
			console.log(res.address, typeof res.family);
		}).catch((e) => { throw e; });
	`)
	if !strings.Contains(out, "number") {
		t.Fatalf("promises lookup failed: %q", out)
	}
	if !strings.Contains(out, "127.0.0.1") && !strings.Contains(out, "::1") {
		t.Fatalf("promises lookup wrong address: %q", out)
	}
}

// TestDNSError checks that a lookup that cannot resolve surfaces an Error with a
// code rather than hanging the loop. The reserved .invalid TLD never resolves.
func TestDNSError(t *testing.T) {
	out := runToEnd(t, `
		const dns = require("node:dns");
		dns.lookup("no-such-host.invalid", (err) => {
			if (!err) { console.log("no error"); return; }
			console.log("err", typeof err.code, err.code.length > 0);
		});
	`)
	if !strings.Contains(out, "err string true") {
		t.Fatalf("expected dns error with code, got: %q", out)
	}
}
