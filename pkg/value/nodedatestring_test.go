package value

// The Date string formats are held against Node rather than against a reading of the
// specification, because the part of them the specification leaves to the implementation
// is the part most likely to be wrong: the long zone name in parentheses is CLDR data
// that no tzdata carries, and the only way to know bento spells it the way a program
// expects is to ask the Node the port targets and keep the answer.
//
// testdata/gen_nodedatestring_ref.js builds the reference; each case is one instant in
// one zone, matched by key, so the two sides cannot drift apart silently.

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

// dateStringCase is one recorded reading. The time value rides as a *float64 because the
// invalid case has no number at all and JSON has no NaN, so it arrives as null; toJSON
// is a json.RawMessage for the same reason on the answer side, since null is one of its
// two forms.
type dateStringCase struct {
	Zone         string          `json:"zone"`
	MS           *float64        `json:"ms"`
	ToString     string          `json:"toString"`
	ToDateString string          `json:"toDateString"`
	ToTimeString string          `json:"toTimeString"`
	ToUTCString  string          `json:"toUTCString"`
	ToISOString  string          `json:"toISOString"`
	ToJSON       json.RawMessage `json:"toJSON"`
}

// knownNameDivergence is the one place bento knowingly prints a different zone name
// than Node, and it is recorded rather than hidden because the recorded answer is one
// Node contradicts itself on. For an instant a second before the epoch, ICU labels these
// two southern zones daylight while printing the standard offset: Node writes
// "GMT+1000 (Australian Eastern Daylight Time)" where Sydney's daylight side is +1100
// and tzdata says the instant is standard time. bento follows the tzdata flag, which is
// the same flag the offset it prints comes from, so its two halves agree.
//
// The comparison is skipped only for the name; every other format in these cases still
// has to match, so a real regression in them is still caught here.
var knownNameDivergence = map[string]bool{
	"Australia/Sydney / before epoch": true,
	"Pacific/Chatham / before epoch":  true,
}

// TestDateStringsMatchNode walks every recorded case, puts the runtime in the zone the
// case was read in, and compares all six formats. A zone the host has no data for skips
// that case rather than failing, since a machine with a trimmed tzdata is a real thing
// and the formats it can read still say whether the code is right.
func TestDateStringsMatchNode(t *testing.T) {
	raw, err := os.ReadFile("testdata/nodedatestring_node24.json")
	if err != nil {
		t.Fatalf("read reference: %v", err)
	}
	var cases map[string]dateStringCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the reference file holds no cases")
	}
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			loc, err := time.LoadLocation(want.Zone)
			if err != nil {
				t.Skipf("this host has no data for %s: %v", want.Zone, err)
			}
			time.Local = loc

			ms := math.NaN()
			if want.MS != nil {
				ms = *want.MS
			}
			d := &Date{ms: ms}

			for _, f := range []struct {
				what  string
				got   func() BStr
				want  string
				named bool // whether this format ends in the parenthesized zone name
			}{
				{"toString", d.ToString, want.ToString, true},
				{"toDateString", d.ToDateString, want.ToDateString, false},
				{"toTimeString", d.ToTimeString, want.ToTimeString, true},
				{"toUTCString", d.ToUTCString, want.ToUTCString, false},
			} {
				got := f.got().ToGoString()
				if f.named && knownNameDivergence[name] {
					if beforeName(got) != beforeName(f.want) {
						t.Errorf("%s = %q, want %q up to the zone name", f.what, got, f.want)
					}
					continue
				}
				if got != f.want {
					t.Errorf("%s = %q, want %q", f.what, got, f.want)
				}
			}

			// toISOString throws for a date with no representable instant, which the
			// reference records as the marker rather than as text, so the throw is part
			// of the comparison instead of a case the test has to know about.
			iso := "<throws>"
			func() {
				defer func() { _ = recover() }()
				iso = d.ToISOString().ToGoString()
			}()
			if iso != want.ToISOString {
				t.Errorf("toISOString = %q, want %q", iso, want.ToISOString)
			}

			gotJSON, err := json.Marshal(jsonReadable(d.ToJSON()))
			if err != nil {
				t.Fatalf("marshal toJSON: %v", err)
			}
			if string(gotJSON) != string(want.ToJSON) {
				t.Errorf("toJSON = %s, want %s", gotJSON, want.ToJSON)
			}
		})
	}
}

// beforeName is a reading with its parenthesized zone name cut off, which is everything
// the known divergence above leaves comparable: the calendar, the clock and the offset.
func beforeName(s string) string {
	if i := strings.LastIndex(s, " ("); i >= 0 {
		return s[:i]
	}
	return s
}

// jsonReadable turns the two forms toJSON answers into something encoding/json spells
// the way the reference file spells them: the null and the ISO string.
func jsonReadable(v Value) any {
	if v.kind == KindNull {
		return nil
	}
	return v.AsString().ToGoString()
}

// TestAnUnnamedZoneFallsBackToTheOffset pins the one reading that is bento's own rather
// than Node's. Go reports a local zone it cannot name as "Local", which is in no CLDR
// table, and the format still has to produce a whole reading: the offset is right and
// the parenthesized name spells the offset again, which is what Node prints for a zone
// CLDR genuinely does not name.
func TestAnUnnamedZoneFallsBackToTheOffset(t *testing.T) {
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	time.Local = time.FixedZone("Local", -7*3600)

	got := (&Date{ms: 0}).ToString().ToGoString()
	want := "Wed Dec 31 1969 17:00:00 GMT-0700 (GMT-07:00)"
	if got != want {
		t.Fatalf("toString in an unnamed zone = %q, want %q", got, want)
	}
}

// TestEveryZoneTheTableNamesIsReadable pins that the generated table is self-consistent:
// every index it holds points at a name that exists, and no zone maps to a pair that
// would read out of range. A generator that renumbered its interning would otherwise
// produce a table that only fails on the one zone a test happens to run in.
func TestEveryZoneTheTableNamesIsReadable(t *testing.T) {
	if len(zoneLongNames) < 100 {
		t.Fatalf("the zone table holds %d zones, want the full tzdata list", len(zoneLongNames))
	}
	for zone, pair := range zoneLongNames {
		if int(pair.standard) >= len(zoneNameText) || int(pair.daylight) >= len(zoneNameText) {
			t.Fatalf("%s names indices %d and %d, past the %d names the table carries",
				zone, pair.standard, pair.daylight, len(zoneNameText))
		}
	}
	if zoneNameText[0] != "" {
		t.Fatalf("index zero is %q, want the empty name an unnamed zone reads", zoneNameText[0])
	}
}
