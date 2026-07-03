// Package anyfixture is a go: import fixture for the section 6.12 any crossing. It
// models the two shapes a real library uses an interface{} for: a passthrough that
// stores and returns a value without inspecting it, and an inspector that type
// switches on the concrete value it received. Echo proves a bento value crosses into
// a Go any and back keeping its identity, and Name proves a bento scalar unwraps to
// the Go native a type switch matches.
package anyfixture

// Echo returns its argument unchanged, the passthrough shape a generic container has.
// A bento value handed in as any and returned keeps its identity, so a round trip
// through Echo is observable back on the bento side as the value that went in.
func Echo(v any) any {
	return v
}

// Name reports the Go kind an any holds after the crossing unwrapped a bento scalar
// to its Go native, so a round trip is observable as the concrete case a Go type
// switch takes: a number crosses as float64, a string as a Go string, and a boolean
// as a Go bool.
func Name(v any) string {
	switch v.(type) {
	case nil:
		return "nil"
	case bool:
		return "bool"
	case float64:
		return "number"
	case string:
		return "string"
	default:
		return "other"
	}
}
