package lower

import (
	"go/token"
	"strconv"
	"testing"
)

// TestDecodeNumericLiteral pins the numeric-literal decoder over the forms this
// slice lowers: decimal integers and fractions, hex, binary, and octal integers,
// underscore separators (stripped), leading- and trailing-dot fractions, and
// exponents. Each case checks the cleaned Go literal text and the token kind, and
// then reparses the cleaned text as a float64 to confirm it names the value the
// same source parses to in JavaScript.
func TestDecodeNumericLiteral(t *testing.T) {
	cases := []struct {
		in    string
		value string
		kind  token.Token
		num   float64
	}{
		{"123", "123", token.INT, 123},
		{"1.5", "1.5", token.FLOAT, 1.5},
		{"0", "0", token.INT, 0},
		{"0xFF", "0xFF", token.INT, 255},
		{"0Xff", "0Xff", token.INT, 255},
		{"0b1010", "0b1010", token.INT, 10},
		{"0o17", "0o17", token.INT, 15},
		{"1_000_000", "1000000", token.INT, 1000000},
		{"0xFF_FF", "0xFFFF", token.INT, 65535},
		{"1e3", "1e3", token.FLOAT, 1000},
		{"1.5e-2", "1.5e-2", token.FLOAT, 0.015},
		{".5", ".5", token.FLOAT, 0.5},
		{"5.", "5.", token.FLOAT, 5},
		{"9007199254740993", "9007199254740993", token.INT, 9007199254740992}, // 2^53+1 rounds down
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			value, kind, ok := decodeNumericLiteral(c.in)
			if !ok {
				t.Fatalf("decodeNumericLiteral(%q) returned ok=false", c.in)
			}
			if value != c.value || kind != c.kind {
				t.Errorf("decodeNumericLiteral(%q) = (%q, %v), want (%q, %v)", c.in, value, kind, c.value, c.kind)
			}
			// The cleaned text must parse to the expected float64 value.
			got, err := parseGoNumber(value)
			if err != nil {
				t.Fatalf("cleaned literal %q does not parse: %v", value, err)
			}
			if got != c.num {
				t.Errorf("value of %q = %v, want %v", c.in, got, c.num)
			}
		})
	}
}

// TestDecodeNumericLiteralRejects pins the literals the decoder hands back: a
// BigInt, which is a different type, and a decimal that overflows to Infinity,
// which Go would reject as an overflowing constant.
func TestDecodeNumericLiteralRejects(t *testing.T) {
	for _, in := range []string{
		"123n",  // BigInt literal
		"0xFFn", // BigInt in hex
		"1e400", // overflows to Infinity
		"",      // empty
	} {
		if value, _, ok := decodeNumericLiteral(in); ok {
			t.Errorf("decodeNumericLiteral(%q) = (%q, ok), want refused", in, value)
		}
	}
}

// parseGoNumber parses a Go integer or float literal to the float64 it denotes,
// covering the radix prefixes the decoder passes through so the tests can confirm
// the value, not just the spelling.
func parseGoNumber(s string) (float64, error) {
	if len(s) >= 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X', 'b', 'B', 'o', 'O':
			i, err := strconv.ParseInt(s, 0, 64)
			if err != nil {
				return 0, err
			}
			return float64(i), nil
		}
	}
	return strconv.ParseFloat(s, 64)
}
