package value

import (
	"math"
	"strings"
)

// This file is a port of node's lib/internal/assert/assertion_error.js, the error
// every assert method throws and, more to the point, the message it carries. An
// assertion message is the whole value of an assertion library: a test that fails
// prints one string, and whether that string says which property differed decides
// whether the failure is diagnosable. So the message is ported line for line rather
// than approximated, down to where the blank lines go.
//
// Four things node does are deliberately not here.
//
// Colors are not carried, the same reason as in the Myers diff: node's color slots
// are empty unless stderr is a color TTY, and a bento binary is the uncolored case,
// which is exactly what node prints into a pipe or under NO_COLOR. The stacked diff's
// terminal width follows from that: node reads process.stderr.columns when stderr is
// a TTY and uses 80 otherwise, and 80 is the width used here.
//
// The stack is not carried. Node captures a stack starting at the assert method, so
// the first frame of an assertion failure is the caller's line; a bento error carries
// no stack at all yet, so err.stack is the "Name: message" line and nothing else.
//
// copyError is not carried. Node duplicates two Error operands before inspecting
// them so the printed forms do not each drag in a stack; a bento error's boxed form
// has no stack property, so there is nothing to hide and the copy would change
// nothing.
//
// The details option is not carried. It is the multi-error form node's internal
// callers use, and no public assert method passes it.

// kReadableOperator is node's table of the same name: the sentence that opens the
// message for each operator. The operator a method reports and the operator this
// table is keyed by are not always the same word, since checkOperator upgrades two
// of them, so the table is keyed by the printing operator.
var kReadableOperator = map[string]string{
	"deepStrictEqual":        "Expected values to be strictly deep-equal:",
	"partialDeepStrictEqual": "Expected values to be partially and strictly deep-equal:",
	"strictEqual":            "Expected values to be strictly equal:",
	"strictEqualObject":      `Expected "actual" to be reference-equal to "expected":`,
	"deepEqual":              "Expected values to be loosely deep-equal:",
	"notDeepStrictEqual":     `Expected "actual" not to be strictly deep-equal to:`,
	"notStrictEqual":         `Expected "actual" to be strictly unequal to:`,
	"notStrictEqualObject":   `Expected "actual" not to be reference-equal to "expected":`,
	"notDeepEqual":           `Expected "actual" not to be loosely deep-equal to:`,
	"notIdentical":           "Values have same structure but are not reference-equal:",
	"notDeepEqualUnequal":    "Expected values not to be loosely deep-equal:",
}

// The two message-length limits node works to. The short one is the width under
// which a difference prints on one line as "actual !== expected" instead of as a
// two-line diff; the long one is where a single inspected value is cut off.
const (
	kMaxShortStringLength = 12
	kMaxLongStringLength  = 512
)

// kAssertTerminalWidth is the width the stacked diff measures against when deciding
// whether to point at the first differing character. Node reads the terminal's own
// width when stderr is a TTY and falls back to 80, and this port is always the
// fallback, since it does not read the terminal.
const kAssertTerminalWidth = 80

// kMethodsWithCustomMessageDiff are the operators whose message keeps its diff even
// when the caller passed a message of its own. For every other operator a custom
// message replaces the message entirely; for these three it becomes the first line
// and the diff still follows, because a diff is the part a caller cannot write.
var kMethodsWithCustomMessageDiff = map[string]bool{
	"deepStrictEqual":        true,
	"strictEqual":            true,
	"partialDeepStrictEqual": true,
}

// assertInspectValue is node's inspectValue: the rendering every value in an
// assertion message goes through. The options are not the console's. Depth is
// effectively unlimited and no array is elided, because a message that says
// "[Object]" where the difference is tells the reader nothing; entries are sorted so
// that two objects whose properties were inserted in different orders diff as equal
// rather than as reordered; getters are read because the comparison reads them too;
// and compact is off, which is what puts every property on its own line and makes a
// line-by-line diff meaningful in the first place.
func assertInspectValue(v Value) string {
	o := defaultInspectOptions()
	o.compactKind = compactFalse
	o.depth = 1000
	o.maxArrayLength = math.MaxInt
	o.sorted = true
	o.getters = gettersAll
	return inspectWith(o, v)
}

// assertGetErrorMessage is node's getErrorMessage: the caller's message when there is
// one, otherwise the sentence for the operator. Node writes it as `message || ...`, so
// a caller's message has to be truthy to win, and hasCustom is that truthiness rather
// than the message's presence.
func assertGetErrorMessage(operator, customMessage string, hasCustom bool) string {
	if hasCustom {
		return customMessage
	}
	return kReadableOperator[operator]
}

// assertIsObject is node's `typeof v === 'object' && v !== null`, which the operator
// upgrades and the comma-disparity check both turn on. An array and a boxed regexp or
// error are objects here, the way typeof reports them.
func assertIsObject(v Value) bool {
	return v.kind == KindObject || v.kind == KindArray
}

// assertCheckOperator is node's checkOperator: two objects or two functions that are
// not the same reference failed a === for one reason only, and saying "expected
// values to be strictly equal" about two objects that print identically reads as a
// bug in the assertion library. The upgraded operator says reference-equal instead.
func assertCheckOperator(actual, expected Value, operator string) string {
	if operator != "strictEqual" {
		return operator
	}
	if (assertIsObject(actual) && assertIsObject(expected)) ||
		(actual.kind == KindFunc && expected.kind == KindFunc) {
		return "strictEqualObject"
	}
	return operator
}

// assertIsSimpleDiff is node's isSimpleDiff: a diff is simple when neither side took
// more than one line to print and at least one side is not an object. The second half
// is what keeps two one-line objects on the diff path, since "{ a: 1 } !== { a: 2 }"
// hides which property moved.
func assertIsSimpleDiff(actual Value, splitActual []string, expected Value, splitExpected []string) bool {
	if len(splitActual) > 1 || len(splitExpected) > 1 {
		return false
	}
	return !assertIsObject(actual) || !assertIsObject(expected)
}

// assertStackedDiff is node's getStackedDiff: the two values on their own lines with
// a caret under the first character they differ at.
//
// The caret is skipped for the first three characters, since a difference that early
// is already visible and the quote a string prints with takes one of them. It is also
// skipped when the two values together are wider than the terminal, where a column
// count no longer points at anything the reader can see.
//
// Node reaches this with two already-inspected values, so its check that both sides
// are strings is always true here and the caret depends on width alone: an assertion
// over two long numbers points at the digit that differs, the same as one over two
// strings.
func assertStackedDiff(actual, expected string) string {
	message := "\n+ " + actual + "\n- " + expected
	if u16Len(actual)+u16Len(expected) > kAssertTerminalWidth {
		return message
	}

	a, e := utf16Units(actual), utf16Units(expected)
	indicatorIdx := -1
	for i := 0; i < len(a); i++ {
		if i >= len(e) || a[i] != e[i] {
			if i >= 3 {
				indicatorIdx = i
			}
			break
		}
	}
	if indicatorIdx == -1 {
		return message
	}
	return message + "\n" + strings.Repeat(" ", indicatorIdx+2) + "^"
}

// assertSimpleDiff is node's getSimpleDiff. It reports the message and, when it wants
// to replace the caller's header, the header to use instead.
//
// Two short values print on one line as "actual !== expected", with no header, since
// a header and a two-line diff around six characters is all frame and no picture. The
// quotes a string prints with do not count toward that width, so a pair of strings
// gets the same room as a pair of numbers. Two zeroes are the exception: 0 and -0 are
// the one pair whose printed forms are nearly identical and whose difference is the
// whole point, so they take the stacked form.
//
// Node has a third path here, a character-level diff for two strings, which it only
// takes when colors are on, since without color it would print the two strings run
// together with no way to tell which characters came from which side.
func assertSimpleDiff(originalActual Value, actual string, originalExpected Value, expected string) (message, header string, hasHeader bool) {
	stringsLen := u16Len(actual) + u16Len(expected)
	if originalActual.kind == KindString {
		stringsLen -= 2
	}
	if originalExpected.kind == KindString {
		stringsLen -= 2
	}
	zero := Number(0)
	bothZero := StrictEquals(originalActual, zero) && StrictEquals(originalExpected, zero)
	if stringsLen <= kMaxShortStringLength && !bothZero {
		return actual + " !== " + expected, "", true
	}
	return assertStackedDiff(actual, expected), "", false
}

// assertErrDiff is node's createErrDiff: the whole message for an operator whose
// failure is a difference between two values. It is the header sentence, then a line
// naming which side is which, then the diff.
//
// Three shapes come out of it. Two values that each print on one line take the simple
// diff above. Two that print identically but are not the same reference are not a
// difference at all, so the operator changes to notIdentical and one copy prints.
// Everything else is a line-by-line Myers diff of the two inspected forms.
func assertErrDiff(actual, expected Value, operator string, customMessage string, hasCustom bool) string {
	operator = assertCheckOperator(actual, expected, operator)

	skipped := false
	message := ""
	inspectedActual := assertInspectValue(actual)
	inspectedExpected := assertInspectValue(expected)
	splitActual := strings.Split(inspectedActual, "\n")
	splitExpected := strings.Split(inspectedExpected, "\n")
	header := "+ actual - expected"

	switch {
	case assertIsSimpleDiff(actual, splitActual, expected, splitExpected):
		simple, simpleHeader, hasSimpleHeader := assertSimpleDiff(actual, splitActual[0], expected, splitExpected[0])
		message = simple
		if hasSimpleHeader {
			header = simpleHeader
		}
	case inspectedActual == inspectedExpected:
		// Structurally the same and not the same reference: there is nothing to diff, so
		// the value prints once. A very long one is cut off at fifty lines, since the
		// reader is being shown the value only to recognize it.
		operator = "notIdentical"
		if len(splitActual) > 50 {
			message = strings.Join(splitActual[:50], "\n") + "\n...}"
			skipped = true
		} else {
			message = inspectedActual
		}
		header = ""
	default:
		// The comma disparity is checked only when the actual side is an object, which is
		// when the inspected form has the trailing commas that would otherwise show as
		// changed lines.
		checkCommaDisparity := assertIsObject(actual)
		diff := myersDiff(splitActual, splitExpected, checkCommaDisparity)
		message, skipped = printMyersDiff(diff)
		// Node replaces the header here for partialDeepStrictEqual, to gray out the actual
		// side of a comparison that only asks the expected side to be present. Uncolored
		// its replacement spells the same "+ actual - expected", so there is nothing to
		// replace.
	}

	headerMessage := assertGetErrorMessage(operator, customMessage, hasCustom) + "\n" + header
	skippedMessage := ""
	if skipped {
		skippedMessage = "\n... Skipped lines"
	}
	return headerMessage + skippedMessage + "\n" + message + "\n"
}

// assertionMessage is the message half of node's AssertionError constructor: the
// branch on operator and on whether the caller wrote a message of its own.
//
// A caller's message replaces the generated one, except for the three operators whose
// diff survives it. Without one, the message is generated three ways: the operators
// that compare take the diff above, the two "not equal" operators print the single
// value that was not supposed to match, and the rest print both values with the
// relation between them spelled out.
func assertionMessage(actual, expected Value, operator string, message Value, hasMessage bool) string {
	if hasMessage {
		custom := ToString(message).ToGoString()
		if kMethodsWithCustomMessageDiff[operator] {
			// The message wins the header only when it is truthy, which is node's `||`: an
			// empty custom message leaves the operator's own sentence in place, and the diff
			// follows either way.
			return assertErrDiff(actual, expected, operator, custom, ToBoolean(message))
		}
		return custom
	}

	if kMethodsWithCustomMessageDiff[operator] {
		return assertErrDiff(actual, expected, operator, "", false)
	}

	if operator == "notDeepStrictEqual" || operator == "notStrictEqual" {
		return assertNotEqualMessage(actual, operator)
	}
	return assertRelationMessage(actual, expected, operator)
}

// assertNotEqualMessage is the branch for notDeepStrictEqual and notStrictEqual: the
// two values matched when they were required not to, so printing both would print the
// same thing twice and only one is shown.
//
// A one-line value joins the sentence, on the same line when it is very short and
// after a blank line when it is not, which is what keeps "not to be strictly equal
// to: 1" on one line while giving a longer value room. A value over fifty lines is
// cut at forty-seven with a "..." standing for the rest.
func assertNotEqualMessage(actual Value, operator string) string {
	base := kReadableOperator[operator]
	res := strings.Split(assertInspectValue(actual), "\n")

	// An object or a function that compared equal under notStrictEqual was the same
	// reference, so the sentence says reference-equal rather than equal.
	if operator == "notStrictEqual" && (assertIsObject(actual) || actual.kind == KindFunc) {
		base = kReadableOperator["notStrictEqualObject"]
	}

	if len(res) > 50 {
		res[46] = "..."
		res = res[:47]
	}

	if len(res) == 1 {
		sep := " "
		if u16Len(res[0]) > 5 {
			sep = "\n\n"
		}
		return base + sep + res[0]
	}
	return base + "\n\n" + strings.Join(res, "\n") + "\n"
}

// assertRelationMessage is the branch for every other operator: both values print,
// with the relation that failed between them. deepEqual and notDeepEqual have a
// sentence for it; the rest, including the "==" that assert.ok and assert.equal
// report, have only the operator itself, which is where "1 == 2" comes from.
//
// Each side is cut at 512 characters, since two values printing in full is already
// the verbose case and a message is still meant to be read.
func assertRelationMessage(actual, expected Value, operator string) string {
	res := assertInspectValue(actual)
	other := assertInspectValue(expected)
	knownOperator := kReadableOperator[operator]

	if operator == "notDeepEqual" && res == other {
		res = knownOperator + "\n\n" + res
		if u16Len(res) > 1024 {
			res = u16Truncate(res, 1021) + "..."
		}
		return res
	}

	if u16Len(res) > kMaxLongStringLength {
		res = u16Truncate(res, 509) + "..."
	}
	if u16Len(other) > kMaxLongStringLength {
		other = u16Truncate(other, 509) + "..."
	}
	switch {
	case operator == "deepEqual":
		res = knownOperator + "\n\n" + res + "\n\nshould loosely deep-equal\n\n"
	case kReadableOperator[operator+"Unequal"] != "":
		res = kReadableOperator[operator+"Unequal"] + "\n\n" + res + "\n\nshould not loosely deep-equal\n\n"
	default:
		other = " " + operator + " " + other
	}
	return res + other
}

// NewAssertionError builds the error an assert method throws. The message follows
// from the operator and the two values the way node's does, and the properties a
// caller reads off a caught assertion are all here: the code it branches on, the two
// values and the operator, and whether the message was generated or its own.
//
// A caught AssertionError does not print the way node's does. Node marks name and
// message non-enumerable, so console.log of one shows the diff and the four
// assertion properties; bento's boxed error carries name and message as ordinary
// properties, so they show too. That is the error box's own divergence rather than
// assert's, and it is one place rather than eleven.
func NewAssertionError(actual, expected Value, operator string, message Value, hasMessage bool) *Error {
	e := NewNodeError("AssertionError", "ERR_ASSERTION", FromGoString(assertionMessage(actual, expected, operator, message, hasMessage)))
	// generatedMessage is node's `!message`, a truthiness test rather than a presence
	// one, so assert.strictEqual(1, 2, '') reports a generated message even though the
	// caller passed one. That is node's own behavior and a program that reads the flag
	// reads it there too.
	e.SetProperty("generatedMessage", Bool(!hasMessage || !ToBoolean(message)))
	e.SetProperty("actual", actual)
	e.SetProperty("expected", expected)
	e.SetProperty("operator", StringValue(FromGoString(operator)))
	// diff is the option the Assert class carries; assert's own methods leave it at
	// "simple", which is the only value this port implements.
	e.SetProperty("diff", StringValue(FromGoString("simple")))
	return e
}

// utf16Units is the code-unit view of a Go string, the indexing JavaScript's s[i]
// does. The caret in a stacked diff is placed by counting units, so a difference
// after a character outside the basic plane lands where node puts it.
func utf16Units(s string) []uint16 {
	return FromGoString(s).units()
}

// u16Truncate cuts a string to a length in UTF-16 code units, node's
// StringPrototypeSlice(s, 0, n). A cut that lands between the halves of a surrogate
// pair leaves a lone surrogate in node; here it takes the replacement character,
// which is the transcoding this port does not carry rather than a choice.
func u16Truncate(s string, n int) string {
	if u16Len(s) <= n {
		return s
	}
	return FromGoString(s).Slice(0, float64(n)).ToGoString()
}
