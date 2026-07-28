package value

import (
	"math/big"
	"testing"
)

// TestNumberObjectBigInt pins new Number(bigint): the constructor takes the real
// value of a bigint through BigIntToNumber, so new Number(0n) wraps +0 and coerces
// back to 0 rather than throwing the TypeError the bare ToNumber raises on a bigint.
// A large bigint past 2^53 rounds to the nearest double the same way Number(b) does.
func TestNumberObjectBigInt(t *testing.T) {
	zero := NumberObject(BigIntValue(&BigInt{}))
	if got := ToNumber(zero); got != 0 {
		t.Fatalf("new Number(0n) wrapped %v, want 0", got)
	}
	big53 := &BigInt{}
	big53.i.Exp(big.NewInt(2), big.NewInt(53), nil) // 2^53
	wrapped := NumberObject(BigIntValue(big53))
	if got := ToNumber(wrapped); got != 9007199254740992 {
		t.Fatalf("new Number(2n**53n) wrapped %v, want 9007199254740992", got)
	}
}
