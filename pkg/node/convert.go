package node

import "math"

// str reads the i-th host-call argument as a string. Missing or non-string
// arguments come back as the empty string so host functions can stay terse.
func str(args []any, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	switch s := args[i].(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case nil:
		return ""
	default:
		return ""
	}
}

// intArg reads the i-th argument as an int. JavaScript numbers arrive as
// float64; integer kinds are handled too.
func intArg(args []any, i int) int {
	if i < 0 || i >= len(args) {
		return 0
	}
	switch n := args[i].(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case int32:
		return int(n)
	default:
		return 0
	}
}

// boolArg reads the i-th argument as a bool using JavaScript truthiness for the
// values the bridge produces.
func boolArg(args []any, i int) bool {
	if i < 0 || i >= len(args) {
		return false
	}
	switch b := args[i].(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case string:
		return b != ""
	default:
		return args[i] != nil
	}
}
