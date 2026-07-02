package value

import (
	"io"
	"os"
	"strings"
)

// This file is the value-model side of the process output streams. The AOT
// compiler lowers process.stdout.write and process.stderr.write to these, so a
// compiled program writes through the same byte path a Go program would, with no
// engine or event loop in between. Node's write returns a boolean that reports
// whether the chunk was accepted by the stream; a synchronous write to a file
// descriptor always is, so both return true.

// WriteStdout writes a string to standard output, the lowering of
// process.stdout.write(chunk). The string is transcoded to its UTF-8 view, the
// byte sink a stream write expects, which maps a lone surrogate to U+FFFD the
// same way Node does when a string is written to a byte stream.
func WriteStdout(s BStr) bool {
	_, _ = os.Stdout.WriteString(s.ToGoString())
	return true
}

// WriteStderr is the standard-error companion of WriteStdout, the lowering of
// process.stderr.write(chunk).
func WriteStderr(s BStr) bool {
	_, _ = os.Stderr.WriteString(s.ToGoString())
	return true
}

// ConsoleLog writes one console.log line to standard output: the parts joined by
// a single space and terminated with a newline, the shape Node's console.log
// prints for a list of already-stringified arguments. The compiler stringifies
// each argument at lower time (a number through NumberToString, a boolean through
// BoolToString, a string as itself), so this only has to join and terminate,
// which keeps the byte path identical to what a hand-written Go program would do.
func ConsoleLog(parts ...BStr) {
	writeConsoleLine(os.Stdout, parts)
}

// ConsoleError is the standard-error companion of ConsoleLog, the lowering of
// console.error and console.warn, which Node writes to standard error with the
// same space-joined, newline-terminated shape.
func ConsoleError(parts ...BStr) {
	writeConsoleLine(os.Stderr, parts)
}

// writeConsoleLine joins the parts with a single space, appends a newline, and
// writes the line in one call so a console line is not interleaved with another
// writer between the arguments.
func writeConsoleLine(w io.Writer, parts []BStr) {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p.ToGoString())
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}
