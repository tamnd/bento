package runtime

import (
	"fmt"
	"math"
)

// arg returns the i-th host-call argument or nil when it is missing, so host
// functions can read positional arguments without bounds checks.
func arg(args []any, i int) any {
	if i < 0 || i >= len(args) {
		return nil
	}
	return args[i]
}

// toInt coerces a value that arrived from JavaScript into an int. JavaScript
// numbers cross the bridge as float64, but integer types are handled too.
func toInt(v any) int { return int(toInt64(v)) }

// toInt64 coerces a bridged value into an int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toString coerces a bridged value into a string. Strings pass through and other
// scalars use their default formatting.
func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(s)
	}
}

// toBool coerces a bridged value into a bool using JavaScript truthiness for the
// common cases the bridge produces.
func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case int:
		return b != 0
	case int64:
		return b != 0
	case string:
		return b != ""
	case nil:
		return false
	default:
		return true
	}
}
