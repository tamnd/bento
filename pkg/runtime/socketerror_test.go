package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// runScript runs a script to completion and returns what it printed. It is for
// the tests that drive a client rather than a server, where the loop exits on its
// own once the last handle closes.
func runScript(t *testing.T, script string) string {
	t.Helper()
	var out bytes.Buffer
	rt, err := New(Config{
		Argv:         []string{"bento", "script.ts"},
		BentoVersion: "test",
		Stdout:       &out,
		Stderr:       &out,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	runErr := rt.RunString("script.ts", script)
	_ = rt.Close()
	if runErr != nil {
		t.Fatalf("run: %v (output %q)", runErr, out.String())
	}
	return out.String()
}

// reportError is the tail of every script here: it prints the error object as
// JSON so the test can check the properties a Node program reads rather than
// only the message.
const reportError = `
	function report(e) {
		console.log(JSON.stringify({
			message: e.message, code: e.code, errno: e.errno,
			syscall: e.syscall, address: e.address, port: e.port, hostname: e.hostname,
		}));
	}
`

type socketErrorReport struct {
	Message  string `json:"message"`
	Code     string `json:"code"`
	Errno    int    `json:"errno"`
	Syscall  string `json:"syscall"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Hostname string `json:"hostname"`
}

func parseReport(t *testing.T, out string) socketErrorReport {
	t.Helper()
	var got socketErrorReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	return got
}

// TestConnectRefusedReachesJavaScriptAsNodeReportsIt is the end to end claim: a
// program that catches a connection error sees Node's message and Node's
// properties, not Go's text and not a bare Error.
//
// It also covers the wiring the classification needed, since the code and the
// errno travel from Go to JavaScript as a JSON property bag and there is nowhere
// else that path is exercised.
func TestConnectRefusedReachesJavaScriptAsNodeReportsIt(t *testing.T) {
	port := freePort(t)
	out := runScript(t, fmt.Sprintf(reportError+`
		const net = require("net");
		const socket = net.connect(%d, "127.0.0.1");
		socket.on("error", report);
	`, port))

	got := parseReport(t, out)
	// Real Node v24 prints exactly this, with no description between the code and
	// the address: connect and listen do not agree on that and the difference is
	// visible to a program matching on the message.
	if want := fmt.Sprintf("connect ECONNREFUSED 127.0.0.1:%d", port); got.Message != want {
		t.Errorf("message = %q, want %q", got.Message, want)
	}
	if got.Code != "ECONNREFUSED" {
		t.Errorf("code = %q, want ECONNREFUSED", got.Code)
	}
	if got.Syscall != "connect" {
		t.Errorf("syscall = %q, want connect", got.Syscall)
	}
	if got.Address != "127.0.0.1" || got.Port != port {
		t.Errorf("address = %s:%d, want 127.0.0.1:%d", got.Address, got.Port, port)
	}
	if got.Errno >= 0 {
		t.Errorf("errno = %d, want libuv's negative number", got.Errno)
	}
}

// TestListenOnABoundPortReportsEADDRINUSE covers the other half of the shape, the
// one that does carry the description, against a port this test holds open so the
// failure is certain.
func TestListenOnABoundPortReportsEADDRINUSE(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	out := runScript(t, fmt.Sprintf(reportError+`
		const net = require("net");
		const server = net.createServer();
		server.on("error", report);
		server.listen(%d, "127.0.0.1");
	`, port))

	got := parseReport(t, out)
	if want := "listen EADDRINUSE: address already in use 127.0.0.1:" + strconv.Itoa(port); got.Message != want {
		t.Errorf("message = %q, want %q", got.Message, want)
	}
	if got.Code != "EADDRINUSE" {
		t.Errorf("code = %q, want EADDRINUSE", got.Code)
	}
	if got.Syscall != "listen" {
		t.Errorf("syscall = %q, want listen", got.Syscall)
	}
}

// TestConnectToAnUnresolvableNameReportsENOTFOUND pins the third shape, the one
// that blames a call the program never made. The name is under .invalid, which
// RFC 2606 reserves precisely so it cannot resolve, so this needs no network.
func TestConnectToAnUnresolvableNameReportsENOTFOUND(t *testing.T) {
	out := runScript(t, reportError+`
		const net = require("net");
		const socket = net.connect(80, "no-such-host.invalid");
		socket.on("error", report);
	`)

	got := parseReport(t, out)
	if want := "getaddrinfo ENOTFOUND no-such-host.invalid"; got.Message != want {
		t.Errorf("message = %q, want %q", got.Message, want)
	}
	if got.Code != "ENOTFOUND" {
		t.Errorf("code = %q, want ENOTFOUND", got.Code)
	}
	if got.Syscall != "getaddrinfo" {
		t.Errorf("syscall = %q, want getaddrinfo", got.Syscall)
	}
	if got.Hostname != "no-such-host.invalid" {
		t.Errorf("hostname = %q, want no-such-host.invalid", got.Hostname)
	}
	// The one code whose number is the same everywhere, because it stands for a
	// getaddrinfo result rather than a platform errno.
	if got.Errno != -3008 {
		t.Errorf("errno = %d, want -3008", got.Errno)
	}
}
