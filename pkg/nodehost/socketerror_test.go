package nodehost

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"syscall"
	"testing"
)

// wantSocketErrno is the number Node reports as err.errno for a socket code on
// this platform, derived the same way wantErrno above derives a filesystem one:
// from the platform's own constant off Windows, from libuv's block on it.
func wantSocketErrno(code string) int {
	if runtime.GOOS == "windows" {
		switch code {
		case "ECONNREFUSED":
			return -4078
		case "EADDRINUSE":
			return -4091
		case "EADDRNOTAVAIL":
			return -4090
		case "EACCES":
			return -4092
		case "ETIMEDOUT":
			return -4039
		}
		return 0
	}
	switch code {
	case "ECONNREFUSED":
		return -int(syscall.ECONNREFUSED)
	case "EADDRINUSE":
		return -int(syscall.EADDRINUSE)
	case "EADDRNOTAVAIL":
		return -int(syscall.EADDRNOTAVAIL)
	case "EACCES":
		return -int(syscall.EACCES)
	case "ETIMEDOUT":
		return -int(syscall.ETIMEDOUT)
	}
	return 0
}

// TestClassifySocketNamesNodesCode covers the failures a networking program
// actually meets. The errors are built the way the standard library hands them
// over, an errno inside a *net.OpError, because that is what the classification
// has to see through.
func TestClassifySocketNamesNodesCode(t *testing.T) {
	opErr := func(op string, en syscall.Errno) error {
		return &net.OpError{Op: op, Net: "tcp", Err: os.NewSyscallError(op, en)}
	}
	cases := []struct {
		what string
		err  error
		code string
		desc string
	}{
		{"a refused connection", opErr("connect", syscall.ECONNREFUSED), "ECONNREFUSED", "connection refused"},
		{"a port already bound", opErr("bind", syscall.EADDRINUSE), "EADDRINUSE", "address already in use"},
		{"a privileged port", opErr("bind", syscall.EACCES), "EACCES", "permission denied"},
		{"an address on no interface", opErr("bind", syscall.EADDRNOTAVAIL), "EADDRNOTAVAIL", "address not available"},
		{"a name that does not resolve", &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}, "ENOTFOUND", "unknown node or service"},
		{"a resolver that timed out", &net.DNSError{Err: "timeout", Name: "x.invalid", IsTimeout: true}, "EAI_AGAIN", "temporary failure"},
		{"a dial that ran out of time", &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}, "ETIMEDOUT", "connection timed out"},
	}
	for _, c := range cases {
		got := ClassifySocketError(c.err)
		if got.Code != c.code {
			t.Errorf("%s: Code = %q, want %q", c.what, got.Code, c.code)
		}
		if got.Desc != c.desc {
			t.Errorf("%s: Desc = %q, want %q", c.what, got.Desc, c.desc)
		}
	}
}

// TestClassifySocketReportsNodesErrno pins the numbers for the codes a program is
// most likely to branch on, since err.errno is as much part of the contract as
// err.code and is the half that differs by platform.
func TestClassifySocketReportsNodesErrno(t *testing.T) {
	for _, code := range []string{"ECONNREFUSED", "EADDRINUSE", "EADDRNOTAVAIL", "EACCES", "ETIMEDOUT"} {
		if got, want := uvErrno(code), wantSocketErrno(code); got != want {
			t.Errorf("uvErrno(%q) = %d, want %d", code, got, want)
		}
	}
	// A name that does not resolve is the one code that is the same number
	// everywhere, because it stands for a getaddrinfo result rather than an errno.
	// Node reports -3008 for it on every platform, EAI_NONAME's number, which it
	// relabels ENOTFOUND on the way out.
	if got := uvErrno("ENOTFOUND"); got != -3008 {
		t.Errorf("uvErrno(ENOTFOUND) = %d, want -3008", got)
	}
}

// TestClassifySocketDoesNotGuessFromTheMessage is the regression this whole file
// exists for. The old implementation matched on the text of the Go error, which
// worked by luck on Unix and not at all on Windows, where the Winsock message for
// a port in use reads "Only one usage of each socket address is normally
// permitted". An error whose text says nothing recognizable but whose errno is
// exact must still classify.
func TestClassifySocketDoesNotGuessFromTheMessage(t *testing.T) {
	err := &net.OpError{
		Op:  "listen",
		Err: errors.New("Only one usage of each socket address (protocol/network address/port) is normally permitted."),
	}
	if got := ClassifySocketError(err); got.Code != "UNKNOWN" {
		t.Errorf("text alone classified as %q, want UNKNOWN: the message is not evidence", got.Code)
	}
	withErrno := &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
	if got := ClassifySocketError(withErrno); got.Code != "EADDRINUSE" {
		t.Errorf("errno classified as %q, want EADDRINUSE", got.Code)
	}
}

// TestNetErrorMatchesNodesMessage pins the message shapes against the ones real
// Node produces. Every expectation here was read off Node v24 rather than
// reasoned about, because the shapes are not consistent with each other: a listen
// error carries the description and a connect or a bind does not, and a port of
// zero is left out of both the message and the error object.
func TestNetErrorMatchesNodesMessage(t *testing.T) {
	opErr := func(op string, en syscall.Errno) error {
		return &net.OpError{Op: op, Net: "tcp", Err: os.NewSyscallError(op, en)}
	}
	cases := []struct {
		what    string
		err     error
		call    string
		address string
		port    int
		message string
		props   map[string]any
	}{
		{
			what: "a refused connection", err: opErr("connect", syscall.ECONNREFUSED),
			call: "connect", address: "127.0.0.1", port: 1,
			message: "connect ECONNREFUSED 127.0.0.1:1",
			props: map[string]any{
				"code": "ECONNREFUSED", "errno": wantSocketErrno("ECONNREFUSED"),
				"syscall": "connect", "address": "127.0.0.1", "port": 1,
			},
		},
		{
			what: "a privileged port", err: opErr("bind", syscall.EACCES),
			call: "listen", address: "127.0.0.1", port: 1,
			message: "listen EACCES: permission denied 127.0.0.1:1",
			props: map[string]any{
				"code": "EACCES", "errno": wantSocketErrno("EACCES"),
				"syscall": "listen", "address": "127.0.0.1", "port": 1,
			},
		},
		{
			// Node leaves a port it was never given out of both, so this is the shape
			// of listen(0, host) failing.
			what: "an address on no interface", err: opErr("bind", syscall.EADDRNOTAVAIL),
			call: "listen", address: "203.0.113.99", port: 0,
			message: "listen EADDRNOTAVAIL: address not available 203.0.113.99",
			props: map[string]any{
				"code": "EADDRNOTAVAIL", "errno": wantSocketErrno("EADDRNOTAVAIL"),
				"syscall": "listen", "address": "203.0.113.99",
			},
		},
		{
			what: "a datagram bind", err: opErr("bind", syscall.EACCES),
			call: "bind", address: "127.0.0.1", port: 1,
			message: "bind EACCES 127.0.0.1:1",
			props: map[string]any{
				"code": "EACCES", "errno": wantSocketErrno("EACCES"),
				"syscall": "bind", "address": "127.0.0.1", "port": 1,
			},
		},
		{
			// The call the caller made was connect, and Node blames getaddrinfo
			// anyway, because the failure happened before the connect was attempted.
			what: "a name that does not resolve",
			err:  &net.DNSError{Err: "no such host", Name: "no-such-host.invalid", IsNotFound: true},
			call: "connect", address: "no-such-host.invalid", port: 80,
			message: "getaddrinfo ENOTFOUND no-such-host.invalid",
			props: map[string]any{
				"code": "ENOTFOUND", "errno": -3008,
				"syscall": "getaddrinfo", "hostname": "no-such-host.invalid",
			},
		},
		{
			// Nothing named the failure and there is no address to report, so the Go
			// error's own text is the message and the properties say only what is
			// known, which here is the call.
			what: "a failure with no address and no code", err: errors.New("the socket fell over"),
			call: "listen", address: "", port: 0,
			message: "the socket fell over",
			props:   map[string]any{"syscall": "listen"},
		},
	}
	for _, c := range cases {
		message, props := NetError(c.err, c.call, c.address, c.port)
		if message != c.message {
			t.Errorf("%s: message = %q, want %q", c.what, message, c.message)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(props), &got); err != nil {
			t.Fatalf("%s: props %q: %v", c.what, props, err)
		}
		if len(got) != len(c.props) {
			t.Errorf("%s: props = %v, want %v", c.what, got, c.props)
		}
		for k, want := range c.props {
			// JSON numbers come back as float64, so the two are compared as numbers
			// rather than as values of whatever type the expectation was written in.
			if n, ok := want.(int); ok {
				if f, ok := got[k].(float64); !ok || int(f) != n {
					t.Errorf("%s: props[%q] = %v, want %d", c.what, k, got[k], n)
				}
				continue
			}
			if got[k] != want {
				t.Errorf("%s: props[%q] = %v, want %v", c.what, k, got[k], want)
			}
		}
	}
}
