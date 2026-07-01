package runtime

import (
	"strings"
	"testing"
)

// TestDgramEcho runs a UDP echo server and a client in one script over the
// loopback. The server echoes each datagram back to its sender; the client
// sends one, checks the reply and the rinfo, and closes both so the loop drains.
func TestDgramEcho(t *testing.T) {
	out := runToEnd(t, `
		const dgram = require("node:dgram");
		const server = dgram.createSocket("udp4");
		server.on("message", (msg, rinfo) => {
			server.send(msg, rinfo.port, rinfo.address);
		});
		server.bind(0, "127.0.0.1", () => {
			const port = server.address().port;
			const client = dgram.createSocket("udp4");
			client.on("message", (msg, rinfo) => {
				console.log("reply", msg.toString(), rinfo.family, rinfo.size);
				client.close();
				server.close();
			});
			client.send("ping", port, "127.0.0.1");
		});
	`)
	if !strings.Contains(out, "reply ping IPv4 4") {
		t.Fatalf("dgram echo failed: %q", out)
	}
}

// TestDgramSendCallback checks the optional send completion callback fires.
func TestDgramSendCallback(t *testing.T) {
	out := runToEnd(t, `
		const dgram = require("node:dgram");
		const server = dgram.createSocket("udp4");
		server.on("message", () => { server.close(); });
		server.bind(0, "127.0.0.1", () => {
			const port = server.address().port;
			const client = dgram.createSocket("udp4");
			client.send("hi", port, "127.0.0.1", (err) => {
				console.log("sent", err === null);
				client.close();
			});
		});
	`)
	if !strings.Contains(out, "sent true") {
		t.Fatalf("dgram send callback failed: %q", out)
	}
}
