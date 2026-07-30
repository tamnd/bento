package value

import (
	"strings"
	"unicode"
)

// This file is a port of node's lib/internal/assert/myers_diff.js, the diff behind
// the `+ actual - expected` block an assertion error prints. It is the shortest edit
// script between the two inspected values, line by line, which is what makes an
// assertion failure readable: a one-property difference in a fifty-property object
// shows as one line rather than as two fifty-line objects.
//
// The algorithm is Myers' O(ND) edit graph search. It is ported rather than replaced
// with any other diff because the output is user-visible text: which side a line is
// attributed to, how a run of unchanged lines collapses, and where the "..." goes are
// all decided here, and every one of those is a byte in a message a test prints.
//
// Colors are not carried. Node reads `internal/util/colors`, whose fields are the
// empty string unless stderr is a TTY that supports them, and a bento binary is the
// no-color case: every color slot below is empty, which is exactly what node prints
// under NO_COLOR or into a pipe. printSimpleMyersDiff is left out for the same
// reason, since node only reaches it when colors are on.

// The three edit operations, node's kOperations.
const (
	diffDelete = -1
	diffNop    = 0
	diffInsert = 1
)

// kNopLinesToCollapse is how many unchanged lines print before the rest of a run is
// collapsed. Node's constant, and it decides both the collapse and the shape of the
// two special cases in printMyersDiff, so it is named rather than inlined.
const kNopLinesToCollapse = 5

// diffOp is one entry of the edit script: an operation and the line it applies to.
type diffOp struct {
	op    int
	value string
}

// areLinesEqual is node's areLinesEqual. The comma disparity is what makes a diff of
// two inspected objects readable: the last property of an object has no trailing
// comma, so inserting a property after it changes a line that did not change in any
// way the reader cares about. With the flag set, a line and the same line with a
// comma are the same line.
func areLinesEqual(actual, expected string, checkCommaDisparity bool) bool {
	if actual == expected {
		return true
	}
	if checkCommaDisparity {
		return actual+"," == expected || actual == expected+","
	}
	return false
}

// myersDiff returns the shortest edit script turning expected into actual, as a
// reversed list of operations: node's callers walk it from the end, and so do the
// printers below.
//
// Node's v array is an Int32Array read one index past each end, where JavaScript
// answers undefined and every comparison against it is false. The two reads that can
// go out of range are each in the branch the loop does not take at that edge, which
// is why diffV can answer zero for an out-of-range index without changing a result.
func myersDiff(actual, expected []string, checkCommaDisparity bool) []diffOp {
	actualLength, expectedLength := len(actual), len(expected)
	max := actualLength + expectedLength

	v := make([]int32, 2*max+1)
	var trace [][]int32

	for diffLevel := 0; diffLevel <= max; diffLevel++ {
		// The state of v is cloned per level, since backtracking replays the search.
		trace = append(trace, append([]int32(nil), v...))

		for diagonalIndex := -diffLevel; diagonalIndex <= diffLevel; diagonalIndex += 2 {
			offset := diagonalIndex + max
			var x int
			if diagonalIndex == -diffLevel ||
				(diagonalIndex != diffLevel && diffV(v, offset-1) < diffV(v, offset+1)) {
				x = int(diffV(v, offset+1))
			} else {
				x = int(diffV(v, offset-1)) + 1
			}
			y := x - diagonalIndex

			for x < actualLength && y < expectedLength &&
				areLinesEqual(actual[x], expected[y], checkCommaDisparity) {
				x++
				y++
			}

			v[offset] = int32(x)

			if x >= actualLength && y >= expectedLength {
				return diffBacktrack(trace, actual, expected, checkCommaDisparity)
			}
		}
	}
	// Unreachable: the search always meets the far corner by level max. Node falls off
	// the same loop and returns undefined, which its callers do not handle either.
	return nil
}

// diffV reads node's Int32Array the way JavaScript does at the two edges: an index
// outside the array is not an error. Zero stands in for undefined, which is only ever
// read into a variable the branch taken at that edge does not use.
func diffV(v []int32, i int) int32 {
	if i < 0 || i >= len(v) {
		return 0
	}
	return v[i]
}

// diffBacktrack is node's backtrack: it walks the recorded search states from the
// last level back to the first, emitting the operation each step took.
func diffBacktrack(trace [][]int32, actual, expected []string, checkCommaDisparity bool) []diffOp {
	actualLength, expectedLength := len(actual), len(expected)
	max := actualLength + expectedLength

	x, y := actualLength, expectedLength
	var result []diffOp

	for diffLevel := len(trace) - 1; diffLevel >= 0; diffLevel-- {
		v := trace[diffLevel]
		diagonalIndex := x - y
		offset := diagonalIndex + max

		var prevDiagonalIndex int
		if diagonalIndex == -diffLevel ||
			(diagonalIndex != diffLevel && diffV(v, offset-1) < diffV(v, offset+1)) {
			prevDiagonalIndex = diagonalIndex + 1
		} else {
			prevDiagonalIndex = diagonalIndex - 1
		}

		prevX := int(diffV(v, prevDiagonalIndex+max))
		prevY := prevX - prevDiagonalIndex

		for x > prevX && y > prevY {
			// An unchanged line is reported from the actual side, except when the two
			// differ only by a trailing comma, where the expected side is the one whose
			// punctuation belongs in a diff the reader is meant to follow.
			actualItem := actual[x-1]
			value := actualItem
			if checkCommaDisparity && !strings.HasSuffix(actualItem, ",") {
				value = expected[y-1]
			}
			result = append(result, diffOp{op: diffNop, value: value})
			x--
			y--
		}

		if diffLevel > 0 {
			if x > prevX {
				x--
				result = append(result, diffOp{op: diffInsert, value: actual[x]})
			} else {
				y--
				result = append(result, diffOp{op: diffDelete, value: expected[y]})
			}
		}
	}

	return result
}

// printMyersDiff renders the edit script as the block an assertion error prints, and
// reports whether it collapsed anything, which the caller says out loud in the header.
// The script is walked backwards because that is the order the lines were in.
//
// A run of more than five unchanged lines collapses, with two special cases that
// exist so that collapsing never costs more lines than it saves: a run of six or
// seven prints the tail of the run instead of a "..." line.
//
// Node's version takes the operator as well, to gray out the actual side of a
// partialDeepStrictEqual. With colors off that is the same text, so it is not a
// parameter here.
func printMyersDiff(diff []diffOp) (string, bool) {
	var message strings.Builder
	skipped := false
	nopCount := 0

	for diffIdx := len(diff) - 1; diffIdx >= 0; diffIdx-- {
		operation, value := diff[diffIdx].op, diff[diffIdx].value
		previousOperation := -2 // node's null: no previous entry rather than an operation
		if diffIdx < len(diff)-1 {
			previousOperation = diff[diffIdx+1].op
		}

		// Avoid grouping if only one line would have been grouped otherwise.
		if previousOperation == diffNop && operation != previousOperation {
			switch {
			case nopCount == kNopLinesToCollapse+1:
				message.WriteString("  " + diff[diffIdx+1].value + "\n")
			case nopCount == kNopLinesToCollapse+2:
				message.WriteString("  " + diff[diffIdx+2].value + "\n")
				message.WriteString("  " + diff[diffIdx+1].value + "\n")
			case nopCount >= kNopLinesToCollapse+3:
				message.WriteString("...\n")
				message.WriteString("  " + diff[diffIdx+1].value + "\n")
				skipped = true
			}
			nopCount = 0
		}

		switch operation {
		case diffInsert:
			// Node writes this line twice, once for partialDeepStrictEqual and once for
			// everything else, and the two differ only in color: the partial form is gray
			// and prints a space instead of the plus when colors are on. With colors off
			// both spell "+ value", so there is one branch here.
			message.WriteString("+ " + value + "\n")
		case diffDelete:
			message.WriteString("- " + value + "\n")
		case diffNop:
			if nopCount < kNopLinesToCollapse {
				message.WriteString("  " + value + "\n")
			}
			nopCount++
		}
	}

	return "\n" + strings.TrimRightFunc(message.String(), unicode.IsSpace), skipped
}
