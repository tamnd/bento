// Generates the reference output for the Date string formats in datestring.go.
//
// Run it with the Node the runtime targets and redirect into the JSON:
//
//   node pkg/value/testdata/gen_nodedatestring_ref.js > pkg/value/testdata/nodedatestring_node24.json
//
// Each case is one instant read in one zone, and it records all five formats plus the
// ISO one, so a change to any of them is caught by the same case. The Go test walks the
// same zone list and the same time values, matched by the case key rather than by
// position, so a case added here fails on the Go side until the Go side has it too.
//
// The zones are chosen to cover what makes these formats hard rather than to be a
// sample: a northern and a southern daylight-saving zone so both sides of the switch
// appear, half-hour and three-quarter-hour offsets, the far ends of the offset range, a
// zone CLDR does not name so the GMT fallback is exercised, and an alias spelling since
// a program runs under whatever TZ says.
//
// Every instant stays inside the era both ICU and a Go tzdata agree on. A pre-1900 date
// would be read through each side's own guess at a city's local mean time and would fail
// for a reason that has nothing to do with the formats, so the extreme time values are
// asked in UTC only, where there is no zone data to disagree about.

'use strict';

const ZONES = [
	'UTC',
	'America/New_York',
	'Europe/Paris',
	'Asia/Tokyo',
	'Australia/Sydney',
	'Asia/Kolkata',
	'Asia/Kathmandu',
	'America/St_Johns',
	'Pacific/Chatham',
	'Pacific/Kiritimati',
	'Antarctica/Troll',
	'Etc/GMT+7',
	'US/Eastern',
	'Asia/Ho_Chi_Minh',
];

// The instants every zone is read at. The two mid-season ones put each zone on both
// sides of its daylight switch wherever it has one, and the two boundary ones sit an
// hour either side of the North American spring forward.
const INSTANTS = {
	'epoch': 0,
	'winter': Date.UTC(2023, 0, 15, 3, 4, 5, 6),
	'summer': Date.UTC(2023, 6, 15, 3, 4, 5, 6),
	'before spring forward': Date.UTC(2023, 2, 12, 6, 59, 59, 999),
	'after spring forward': Date.UTC(2023, 2, 12, 7, 0, 0, 0),
	'before epoch': Date.UTC(1969, 11, 31, 23, 59, 59, 1),
	'end of year': Date.UTC(2024, 11, 31, 23, 59, 59, 999),
};

// The instants read in UTC alone: the ends of the representable range, the years whose
// spelling is unusual, and the one value that is not a date at all.
const UTC_ONLY = {
	'max time value': 8.64e15,
	'min time value': -8.64e15,
	'year one': -62135596800000,
	'year zero': -62167219200000,
	'negative year': -62198755200000,
	'five digit year': Date.UTC(12023, 0, 1),
	'invalid': NaN,
};

// formats reads every string form of one date. toJSON is recorded as its JSON form
// rather than as a string, since it answers null for a date with no representable
// instant and the point of recording it is that it does not throw there.
function formats(d) {
	let iso = null;
	try {
		iso = d.toISOString();
	} catch {
		iso = '<throws>';
	}
	return {
		toString: d.toString(),
		toDateString: d.toDateString(),
		toTimeString: d.toTimeString(),
		toUTCString: d.toUTCString(),
		toISOString: iso,
		toJSON: d.toJSON(),
	};
}

const cases = {};
for (const zone of ZONES) {
	process.env.TZ = zone;
	for (const [name, ms] of Object.entries(INSTANTS)) {
		// Node reads TZ per Date construction through ICU's default zone, which it
		// re-reads when the variable changes, so the assignment above is what puts each
		// case in its zone.
		cases[zone + ' / ' + name] = { zone, ms, ...formats(new Date(ms)) };
	}
}
process.env.TZ = 'UTC';
for (const [name, ms] of Object.entries(UTC_ONLY)) {
	cases['UTC / ' + name] = { zone: 'UTC', ms, ...formats(new Date(ms)) };
}

process.stdout.write(JSON.stringify(cases, null, '\t') + '\n');
