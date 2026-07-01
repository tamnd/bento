package engine

import "strings"

// Exception is a JavaScript error that escaped to Go: an uncaught throw from
// Eval, EvalModule, or a Call into JavaScript. Backends return it so the CLI can
// tell a program's own error apart from an internal failure and print it the way
// a JavaScript developer expects.
type Exception struct {
	// Message is the error's string form, for example "Error: kaboom".
	Message string
	// Stack is the captured stack trace when the backend can provide one, else
	// empty.
	Stack string
	// Script is the name of the script that threw, for context.
	Script string
}

// Error implements the error interface.
func (e *Exception) Error() string {
	if e.Stack != "" {
		return e.Stack
	}
	return e.Message
}

// Display returns a Node-style rendering suitable for writing to stderr when an
// exception goes uncaught.
func (e *Exception) Display() string {
	msg := strings.TrimSpace(e.Message)
	if e.Stack != "" {
		return "Uncaught " + strings.TrimSpace(e.Stack)
	}
	return "Uncaught " + msg
}
