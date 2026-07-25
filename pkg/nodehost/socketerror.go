package nodehost

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// ClassifySocketError maps a Go network error to the Node error it stands for.
//
// It is ClassifyFSError's sibling and works the same way, by asking the errno
// rather than reading the message. Reading the message is what bento used to do
// here, matching on "address already in use" and "permission denied", and that
// is wrong twice over on Windows: the text comes from the Winsock error, which
// says "Only one usage of each socket address is normally permitted", so nothing
// matched and every socket error arrived with no code at all.
//
// The errno on Windows is a Winsock number rather than a POSIX one, WSAECONNREFUSED
// being 10061 where POSIX ECONNREFUSED is 111, which is the other half of the same
// problem and is handled where the codes are translated, in the platform files.
//
// Name resolution is asked before the errno rather than after it, which is the
// reverse of ClassifyFSError and matters on Windows. A failed lookup there does
// carry an errno, WSAHOST_NOT_FOUND, and libuv translates that to ENOENT, so
// asking the number first would answer ENOENT where Node says ENOTFOUND. A
// *net.DNSError is unambiguous about what failed, so it decides on its own.
//
// A deadline is the other failure with no errno behind it: Go reports a dial
// timeout through os.ErrDeadlineExceeded and Node calls it ETIMEDOUT.
func ClassifySocketError(err error) UVError {
	code := "UNKNOWN"
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr):
		switch {
		case dnsErr.IsNotFound:
			code = "ENOTFOUND"
		case dnsErr.IsTimeout:
			code = "EAI_AGAIN"
		default:
			code = "EAI_FAIL"
		}
	default:
		if en, ok := errors.AsType[syscall.Errno](err); ok {
			code = uvCode(en)
		}
		if code == "UNKNOWN" && errors.Is(err, os.ErrDeadlineExceeded) {
			code = "ETIMEDOUT"
		}
	}
	return UVError{Code: code, Errno: uvErrno(code), Desc: uvDesc(code, err)}
}

// NetError builds the error a failed network call raises in Node: the message the
// Error carries and the properties Node hangs off it, as a JSON object ready for
// the JavaScript side to assign.
//
// call is the syscall Node blames, "connect" or "listen" or "bind". address and
// port are what the call named; pass an empty address when there is none to
// report, and the Go error's own text stands in for the message, since a Node
// message without an address is not a shape Node produces.
//
// The address is the host the caller asked for rather than the address the
// resolver returned. They differ only when a name resolved and then the connect
// failed, which is rare enough next to the connect-refused case that reporting
// the name is worth more than a second lookup.
func NetError(err error, call, address string, port int) (message, props string) {
	e := ClassifySocketError(err)
	p := map[string]any{}
	if call != "" {
		p["syscall"] = call
	}
	// A failure nothing named gets no code rather than a code of "UNKNOWN". Node
	// leaves err.code off an error it cannot name, and a program testing for the
	// property is better served by absence than by a string that means nothing.
	if e.Code != "UNKNOWN" {
		p["code"] = e.Code
		p["errno"] = e.Errno
	}

	// A name that does not resolve failed before the call the caller made, so Node
	// blames getaddrinfo rather than connect and reports the name as the hostname.
	if e.Code == "ENOTFOUND" || strings.HasPrefix(e.Code, "EAI_") {
		p["syscall"] = "getaddrinfo"
		if address != "" {
			p["hostname"] = address
			return "getaddrinfo " + e.Code + " " + address, jsonObject(p)
		}
		return err.Error(), jsonObject(p)
	}
	// Node's shape is the call, the code and the address. With any of the three
	// missing there is no shape to build, so the Go error's own text is the honest
	// message: it says more than "listen UNKNOWN" would.
	if address == "" || call == "" || e.Code == "UNKNOWN" {
		return err.Error(), jsonObject(p)
	}

	msg := call + " " + e.Code
	if call == "listen" {
		// Node prints the description for a listen error and leaves it out of a
		// connect or a bind. There is no rule behind that, only which internal
		// helper each call site reached for: uvExceptionWithHostPort keeps the
		// description and exceptionWithHostPort drops it. A program matching on the
		// message sees the difference, so bento copies it.
		msg += ": " + e.Desc
	}
	msg += " " + address
	p["address"] = address
	// Port 0 means the caller let the platform choose, and Node leaves a port it
	// does not know out of both the message and the error object.
	if port != 0 {
		msg += ":" + strconv.Itoa(port)
		p["port"] = port
	}
	return msg, jsonObject(p)
}

// jsonObject renders the property bag. The keys are ours and the values are
// strings and ints, so there is nothing here that can fail to encode.
func jsonObject(p map[string]any) string {
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}
