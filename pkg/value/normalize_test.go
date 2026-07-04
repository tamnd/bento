package value

import "testing"

// TestNormalizeForms checks each of the four normalization forms against a known
// pair. The canonical forms move between the precomposed A-with-ring and its A
// plus combining-ring decomposition; the compatibility forms additionally fold
// the fi ligature to the two letters f and i, which the canonical forms leave
// alone.
func TestNormalizeForms(t *testing.T) {
	precomposed := "Å" // A with ring above, one code point
	decomposed := "Å" // A followed by combining ring above
	ligature := "ﬁ"    // the fi ligature, one code point
	cases := []struct {
		form, in, want string
	}{
		{"NFC", decomposed, precomposed},
		{"NFD", precomposed, decomposed},
		{"NFKC", decomposed, precomposed},
		{"NFKD", precomposed, decomposed},
		{"NFC", ligature, ligature},
		{"NFD", ligature, ligature},
		{"NFKC", ligature, "fi"},
		{"NFKD", ligature, "fi"},
	}
	for _, c := range cases {
		if got := bs(c.in).Normalize(bs(c.form)).ToGoString(); got != c.want {
			t.Errorf("Normalize(%q) on %q = %q, want %q", c.form, c.in, got, c.want)
		}
	}
}

// TestNormalizeDefaultsToNFC proves that calling normalize with no form argument
// composes to NFC, the default the method uses when the argument is omitted.
func TestNormalizeDefaultsToNFC(t *testing.T) {
	if got := bs("Å").Normalize().ToGoString(); got != "Å" {
		t.Errorf("Normalize() = %q, want %q", got, "Å")
	}
}

// TestNormalizeBadFormThrows proves a form name that is not one of the four the
// specification allows raises a RangeError rather than normalizing to some default.
func TestNormalizeBadFormThrows(t *testing.T) {
	for _, form := range []string{"NFE", "nfc", "", "NFKD ", "utf8"} {
		func() {
			defer func() {
				r := recover()
				e, ok := r.(*Error)
				if !ok || !e.IsA("RangeError") {
					t.Errorf("Normalize(%q): expected a RangeError, got %v", form, r)
				}
			}()
			bs("abc").Normalize(bs(form))
			t.Errorf("Normalize(%q): expected a throw", form)
		}()
	}
}

// TestNormalizePreservesLoneSurrogate proves the slow path keeps a lone surrogate
// intact while still normalizing the well-formed text on either side of it. The
// input is a decomposed A-with-ring, a lone high surrogate, and a second decomposed
// A-with-ring; NFC composes both letters and leaves the surrogate untouched between
// them.
func TestNormalizePreservesLoneSurrogate(t *testing.T) {
	in := FromUTF16([]uint16{0x0041, 0x030A, 0xD800, 0x0041, 0x030A})
	got := in.Normalize(bs("NFC")).units()
	want := []uint16{0x00C5, 0xD800, 0x00C5}
	if len(got) != len(want) {
		t.Fatalf("Normalize preserved lone surrogate = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Normalize preserved lone surrogate = %v, want %v", got, want)
		}
	}
}

// TestNormalizeIdempotent proves normalizing an already-normalized string returns
// an equal string, so a second pass is a no-op the way the method guarantees.
func TestNormalizeIdempotent(t *testing.T) {
	once := bs("Åcafé").Normalize(bs("NFC"))
	twice := once.Normalize(bs("NFC"))
	if !once.Equal(twice) {
		t.Errorf("Normalize was not idempotent: %q then %q", once.ToGoString(), twice.ToGoString())
	}
}
