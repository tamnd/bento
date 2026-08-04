package value

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// freshConsoleState gives one test its own counts, timers, and indent, and puts
// the package's back when it is done. The console's state is package-level the
// way Node's is per instance, so without this a test that counted would be read
// by the next one that counted.
func freshConsoleState(t *testing.T) {
	t.Helper()
	counts, timers, indent := consoleCounts, consoleTimers, consoleIndent
	t.Cleanup(func() {
		consoleCounts, consoleTimers, consoleIndent = counts, timers, indent
	})
	consoleCounts, consoleTimers, consoleIndent = map[string]int{}, map[string]time.Time{}, ""
}

// captureConsole runs f with both standard streams redirected to a pipe and
// answers what was written to each. A console helper writes to os.Stdout and
// os.Stderr directly, which is the whole point of it (a compiled program's output
// is the process's own bytes), so reading it back means moving the files.
func captureConsole(t *testing.T, f func()) (string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	f()
	os.Stdout, os.Stderr = stdout, stderr
	_ = outW.Close()
	_ = errW.Close()
	out, _ := io.ReadAll(outR)
	errs, _ := io.ReadAll(errR)
	return string(out), string(errs)
}

// warningPrefix is the "(node:pid) Warning: " Node's process.emitWarning puts in
// front of a console warning, spelled with this process's own id.
func warningPrefix() string {
	return "(node:" + strconv.Itoa(os.Getpid()) + ") Warning: "
}

// TestConsoleCountTalliesPerLabel pins the counter console.count keeps. An
// omitted label counts under "default", a label of its own counts on its own,
// and the label is coerced, so 1 and '1' share a tally the way Node's do.
func TestConsoleCountTalliesPerLabel(t *testing.T) {
	freshConsoleState(t)
	out, _ := captureConsole(t, func() {
		ConsoleCount(Undefined)
		ConsoleCount(Undefined)
		ConsoleCount(StringValue(FromGoString("a")))
		ConsoleCount(Number(1))
		ConsoleCount(StringValue(FromGoString("1")))
	})
	want := "default: 1\ndefault: 2\na: 1\n1: 1\n1: 2\n"
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

// TestConsoleCountResetStartsTheTallyOver pins that a reset label counts from one
// again, and that resetting a label nothing counted draws Node's warning rather
// than passing quietly, since it is almost always a typo.
func TestConsoleCountResetStartsTheTallyOver(t *testing.T) {
	freshConsoleState(t)
	out, errs := captureConsole(t, func() {
		ConsoleCount(Undefined)
		ConsoleCountReset(Undefined)
		ConsoleCount(Undefined)
		ConsoleCountReset(StringValue(FromGoString("nope")))
	})
	if want := "default: 1\ndefault: 1\n"; out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
	if want := warningPrefix() + "Count for 'nope' does not exist\n"; errs != want {
		t.Fatalf("want %q, got %q", want, errs)
	}
}

// TestConsoleTimeKeepsLabelsOfDifferentKindsApart pins that the timers are keyed
// by the label value rather than by its string form, which is the difference
// between console.time and console.count: Node holds the timers in a Map, so
// time(1) and time('1') are two timers, while count coerces before it tallies.
func TestConsoleTimeKeepsLabelsOfDifferentKindsApart(t *testing.T) {
	freshConsoleState(t)
	_, errs := captureConsole(t, func() {
		ConsoleTime(Number(1))
		ConsoleTime(StringValue(FromGoString("1")))
	})
	if errs != "" {
		t.Fatalf("two labels of different kinds should be two timers, got warning %q", errs)
	}
	if len(consoleTimers) != 2 {
		t.Fatalf("want two timers, got %d", len(consoleTimers))
	}
}

// TestConsoleTimeWarnsOnALabelAlreadyRunning pins that a second start under one
// label leaves the first alone and warns. Overwriting would silently lose the
// start the program cares about, so Node refuses instead.
func TestConsoleTimeWarnsOnALabelAlreadyRunning(t *testing.T) {
	freshConsoleState(t)
	_, errs := captureConsole(t, func() {
		ConsoleTime(StringValue(FromGoString("t")))
		ConsoleTime(StringValue(FromGoString("t")))
	})
	if want := warningPrefix() + "Label 't' already exists for console.time()\n"; errs != want {
		t.Fatalf("want %q, got %q", want, errs)
	}
	if len(consoleTimers) != 1 {
		t.Fatalf("the second start should have been refused, got %d timers", len(consoleTimers))
	}
}

// TestConsoleTimeEndPrintsAndStopsTheTimer pins the line timeEnd writes and that
// the timer is gone after it, so a second timeEnd warns rather than printing a
// second duration.
func TestConsoleTimeEndPrintsAndStopsTheTimer(t *testing.T) {
	freshConsoleState(t)
	out, errs := captureConsole(t, func() {
		ConsoleTime(StringValue(FromGoString("t")))
		ConsoleTimeEnd(StringValue(FromGoString("t")))
		ConsoleTimeEnd(StringValue(FromGoString("t")))
	})
	if !strings.HasPrefix(out, "t: ") || !strings.HasSuffix(out, "ms\n") {
		t.Fatalf("want a t: <duration>ms line, got %q", out)
	}
	if want := warningPrefix() + "No such label 't' for console.timeEnd()\n"; errs != want {
		t.Fatalf("want %q, got %q", want, errs)
	}
}

// TestConsoleTimeLogLeavesTheTimerRunning pins that timeLog prints the duration
// so far and keeps the timer, which is what lets a program mark several points
// inside one span, and that its extra arguments go through the format pass: a
// specifier in the first of them fills from the ones after it.
func TestConsoleTimeLogLeavesTheTimerRunning(t *testing.T) {
	freshConsoleState(t)
	out, errs := captureConsole(t, func() {
		ConsoleTime(StringValue(FromGoString("t")))
		ConsoleTimeLog(StringValue(FromGoString("t")), StringValue(FromGoString("mark %d")), Number(7))
		ConsoleTimeLog(StringValue(FromGoString("t")))
	})
	if errs != "" {
		t.Fatalf("the timer should still be running, got warning %q", errs)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two lines, got %q", out)
	}
	// The format string is the "%s: %s" the label and the duration fill, so what the
	// call passed after them is appended rather than read as a format of its own.
	// That is Node's own line, log('%s: %s', label, formatted, ...data), and it is
	// why a specifier in the extra arguments stands rather than filling.
	if !strings.HasPrefix(lines[0], "t: ") || !strings.HasSuffix(lines[0], " mark %d 7") {
		t.Fatalf("want the marks appended to the first line, got %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], "ms") {
		t.Fatalf("want a bare duration on the second line, got %q", lines[1])
	}
}

// TestConsoleGroupIndentsWhatFollows pins the indent an open group puts in front
// of every later line, including each line of an argument that spans several, and
// that group prints its own arguments before it indents them.
func TestConsoleGroupIndentsWhatFollows(t *testing.T) {
	freshConsoleState(t)
	out, _ := captureConsole(t, func() {
		ConsoleGroup(FromGoString("outer"))
		ConsoleLog(FromGoString("inside"))
		ConsoleGroup()
		ConsoleLog(FromGoString("two\nlines"))
		ConsoleGroupEnd()
		ConsoleGroupEnd()
		ConsoleLog(FromGoString("back"))
	})
	want := "outer\n  inside\n    two\n    lines\nback\n"
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

// TestConsoleGroupEndAtTheOutermostLevelIsQuiet pins that an unmatched groupEnd
// leaves the indent where it is rather than indenting negatively, so a program
// that closes one group too many still prints its later lines flush left.
func TestConsoleGroupEndAtTheOutermostLevelIsQuiet(t *testing.T) {
	freshConsoleState(t)
	out, _ := captureConsole(t, func() {
		ConsoleGroupEnd()
		ConsoleGroupEnd()
		ConsoleLog(FromGoString("flush"))
	})
	if want := "flush\n"; out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

// TestConsoleAssertReportsOnlyOnAFalseCondition pins both halves of assert: it is
// quiet when the condition holds, and it writes to standard error when it does
// not. It never throws, which is what separates it from assert().
func TestConsoleAssertReportsOnlyOnAFalseCondition(t *testing.T) {
	freshConsoleState(t)
	_, quiet := captureConsole(t, func() {
		ConsoleAssert(Bool(true), StringValue(FromGoString("never")))
	})
	if quiet != "" {
		t.Fatalf("a holding condition should print nothing, got %q", quiet)
	}
	_, errs := captureConsole(t, func() {
		ConsoleAssert(Bool(false))
		ConsoleAssert(Bool(false), StringValue(FromGoString("")))
		ConsoleAssert(Bool(false), StringValue(FromGoString("with %d")), Number(7))
		ConsoleAssert(Bool(false), Number(42), StringValue(FromGoString("tail")))
		ConsoleAssert(Bool(false), Null)
	})
	want := "Assertion failed\n" +
		"Assertion failed: \n" +
		"Assertion failed: with 7\n" +
		"Assertion failed 42 tail\n" +
		"Assertion failed null\n"
	if errs != want {
		t.Fatalf("want %q, got %q", want, errs)
	}
}

// TestConsoleDirQuotesABareString pins the one place dir and log differ on the
// same argument. log prints a string as its own text, since a program's output
// would be unreadable otherwise, and dir inspects it, quotes and all.
func TestConsoleDirQuotesABareString(t *testing.T) {
	freshConsoleState(t)
	out, _ := captureConsole(t, func() {
		ConsoleDir(StringValue(FromGoString("text")))
		ConsoleLog(FromGoString("text"))
	})
	if want := "'text'\ntext\n"; out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

// TestConsoleClearWritesNothingToAPipe pins that clear is a no-op off a terminal.
// The escape sequences it writes mean nothing to a file or a pipe, and a test
// harness reading a program's output would see them as content.
func TestConsoleClearWritesNothingToAPipe(t *testing.T) {
	freshConsoleState(t)
	out, errs := captureConsole(t, ConsoleClear)
	if out != "" || errs != "" {
		t.Fatalf("clear off a terminal should write nothing, got %q and %q", out, errs)
	}
}

// TestConsoleObjectMembersReachTheSameHelpers pins that reading the console as a
// value and calling through it does what calling the method directly does. The
// lowerer emits a direct call when it can see the method name and reaches the
// object otherwise, so the two paths have to agree.
func TestConsoleObjectMembersReachTheSameHelpers(t *testing.T) {
	freshConsoleState(t)
	c := ConsoleObject()
	if c.Kind() != KindObject {
		t.Fatalf("the console should be an object, got kind %v", c.Kind())
	}
	if ConsoleObject() != c {
		t.Fatal("a second read of the console should be the same object")
	}
	call := func(name string, args ...Value) {
		t.Helper()
		m := c.Get(FromGoString(name))
		if m.Kind() != KindFunc {
			t.Fatalf("console.%s should be a function, got kind %v", name, m.Kind())
		}
		m.Call(args...)
	}
	out, errs := captureConsole(t, func() {
		call("log", StringValue(FromGoString("%s!")), StringValue(FromGoString("hi")))
		call("count")
		call("group", StringValue(FromGoString("g")))
		call("dir", StringValue(FromGoString("text")))
		call("groupEnd")
		call("assert", Bool(false), StringValue(FromGoString("no")))
	})
	if want := "hi!\ndefault: 1\ng\n  'text'\n"; out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
	if want := "Assertion failed: no\n"; errs != want {
		t.Fatalf("want %q, got %q", want, errs)
	}
}

// TestFormatConsoleDurationLadder pins the four shapes Node's timeEnd prints a
// span in. The unit changes at a second and again at a minute, and past an hour
// the clock form grows a field, because a bare number of seconds stops being
// readable there.
func TestFormatConsoleDurationLadder(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{1500 * time.Microsecond, "1.500ms"},
		{999 * time.Millisecond, "999.000ms"},
		{2500 * time.Millisecond, "2.500s"},
		{90 * time.Second, "1:30.000 (m:ss.mmm)"},
		{3661500 * time.Millisecond, "1:01:01.500 (h:mm:ss.mmm)"},
	} {
		if got := formatConsoleDuration(c.d); got != c.want {
			t.Fatalf("%v: want %q, got %q", c.d, c.want, got)
		}
	}
}
