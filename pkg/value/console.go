package value

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// This file is the runtime side of the console global. The writing half lives in
// stdout.go, where ConsoleLog and ConsoleError put a finished line on a stream;
// what is here is the rest of the surface, the members that carry state or a
// format of their own.
//
// Three of them keep state between calls. count keeps a tally per label, time
// keeps a start instant per label, and group keeps an indent every later line is
// written under. Node keeps all three on the console instance; a compiled program
// has one console, so they are package-level here, the same way the timer queues
// and the process object are.
//
// The console is also a value. A program that reads the name rather than calling
// through it, globalThis.console = globalThis.console being the plainest case,
// gets the object ConsoleObject builds, whose members are the same helpers the
// lowerer emits a direct call to when it can see the method name. The two paths
// have to agree, so the object's entries call the same functions.

// The console's state. counts is the tally console.count keeps per label, timers
// the start instant console.time keeps, and groupIndent the string console.group
// puts in front of every line until the matching groupEnd. Node's console holds
// each of these per instance; there is one console in a compiled program.
var (
	consoleCounts   = map[string]int{}
	consoleTimers   = map[string]time.Time{}
	consoleIndent   string
	consoleInstance Value
)

// ConsoleObject returns the console global as an object, building it on first
// read and caching it, so console === console holds and a property a program
// sets on it is still there on the next read.
//
// The members are the ones the lowerer also emits direct calls for, wrapped so a
// dynamic call reaches the same helper. They are ordinary enumerable properties,
// unlike the entries on globalThis: Node's console carries its methods as own
// enumerable properties, which is what makes Object.keys(console) list them.
func ConsoleObject() Value {
	if consoleInstance.Kind() != KindUndefined {
		return consoleInstance
	}
	c := NewObject()
	writer := func(w func(...BStr)) Value {
		return NewFunc(func(args []Value) Value {
			w(ConsoleFormat(args...))
			return Undefined
		})
	}
	c.Set(FromGoString("log"), writer(ConsoleLog))
	c.Set(FromGoString("info"), writer(ConsoleLog))
	c.Set(FromGoString("debug"), writer(ConsoleLog))
	c.Set(FromGoString("dir"), NewFunc(func(args []Value) Value {
		ConsoleDir(argAt(args, 0))
		return Undefined
	}))
	c.Set(FromGoString("dirxml"), writer(ConsoleLog))
	c.Set(FromGoString("error"), writer(ConsoleError))
	c.Set(FromGoString("warn"), writer(ConsoleError))
	c.Set(FromGoString("clear"), NewFunc(func(args []Value) Value {
		ConsoleClear()
		return Undefined
	}))
	c.Set(FromGoString("count"), NewFunc(func(args []Value) Value {
		ConsoleCount(argAt(args, 0))
		return Undefined
	}))
	c.Set(FromGoString("countReset"), NewFunc(func(args []Value) Value {
		ConsoleCountReset(argAt(args, 0))
		return Undefined
	}))
	c.Set(FromGoString("group"), NewFunc(func(args []Value) Value {
		ConsoleGroup(ConsoleFormat(args...))
		return Undefined
	}))
	c.Set(FromGoString("groupCollapsed"), NewFunc(func(args []Value) Value {
		ConsoleGroup(ConsoleFormat(args...))
		return Undefined
	}))
	c.Set(FromGoString("groupEnd"), NewFunc(func(args []Value) Value {
		ConsoleGroupEnd()
		return Undefined
	}))
	c.Set(FromGoString("time"), NewFunc(func(args []Value) Value {
		ConsoleTime(argAt(args, 0))
		return Undefined
	}))
	c.Set(FromGoString("timeEnd"), NewFunc(func(args []Value) Value {
		ConsoleTimeEnd(argAt(args, 0))
		return Undefined
	}))
	c.Set(FromGoString("timeLog"), NewFunc(func(args []Value) Value {
		ConsoleTimeLog(argAt(args, 0), rest(args, 1)...)
		return Undefined
	}))
	c.Set(FromGoString("assert"), NewFunc(func(args []Value) Value {
		ConsoleAssert(argAt(args, 0), rest(args, 1)...)
		return Undefined
	}))
	for _, name := range []string{"timeStamp", "profile", "profileEnd"} {
		c.Set(FromGoString(name), NewFunc(func(args []Value) Value {
			ConsoleNoop(args...)
			return Undefined
		}))
	}
	consoleInstance = c
	return consoleInstance
}

// argAt reads one argument out of a dynamic call's argument list, answering
// undefined for a position the caller left off, which is what a JavaScript
// parameter binds when the call is short.
func argAt(args []Value, i int) Value {
	if i < len(args) {
		return args[i]
	}
	return Undefined
}

// rest answers the arguments a call made past position i, and nothing at all
// when the call made none. The difference matters where a member reads the count
// rather than the values: console.assert(false) prints "Assertion failed" and
// console.assert(false, "") prints it with a colon after, so an empty list and a
// list holding one empty string are not the same thing.
func rest(args []Value, i int) []Value {
	if i >= len(args) {
		return nil
	}
	return args[i:]
}

// ConsoleClear is console.clear. On a terminal it moves the cursor home and
// clears the screen below it, the two escape sequences Node writes; on anything
// else, a pipe or a file, it does nothing at all, because there is no screen to
// clear and Node writes nothing.
func ConsoleClear() {
	if !isTerminal(os.Stdout) {
		return
	}
	_, _ = os.Stdout.WriteString("\x1b[1;1H\x1b[0J")
}

// isTerminal reports whether a file is a terminal rather than a pipe or an
// ordinary file, which is what decides whether console.clear has a screen to
// clear. A terminal is a character device, so the file's mode settles it without
// a syscall of its own or a dependency outside the standard library.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// ConsoleCount is console.count. It counts the calls made under a label and
// prints the label and the running total, which is how a program counts times
// through a path without writing the counter itself. An omitted label counts
// under "default", and any other value is counted under its string form, so
// console.count(null) and console.count('null') share a tally the way Node's do.
func ConsoleCount(label Value) {
	name := consoleLabel(label)
	consoleCounts[name]++
	ConsoleLog(FromGoString(name + ": " + strconv.Itoa(consoleCounts[name])))
}

// ConsoleCountReset is console.countReset, which drops one label's tally so the
// next count under it starts at one again. A label that was never counted draws
// the warning Node prints, since resetting a counter that does not exist is
// almost always a typo in the label.
func ConsoleCountReset(label Value) {
	name := consoleLabel(label)
	if _, ok := consoleCounts[name]; !ok {
		consoleWarn("Count for '" + name + "' does not exist")
		return
	}
	delete(consoleCounts, name)
}

// ConsoleTime is console.time, which starts a timer under a label. A label
// already timing is left alone and draws a warning, the way Node refuses to
// restart a running timer rather than silently losing the first start.
func ConsoleTime(label Value) {
	key := consoleTimerKey(label)
	if _, ok := consoleTimers[key]; ok {
		consoleWarn("Label '" + consoleLabel(label) + "' already exists for console.time()")
		return
	}
	consoleTimers[key] = time.Now()
}

// ConsoleTimeEnd is console.timeEnd, which prints how long the label's timer ran
// and stops it. A label with no timer draws a warning and prints nothing.
func ConsoleTimeEnd(label Value) {
	name, key := consoleLabel(label), consoleTimerKey(label)
	started, ok := consoleTimers[key]
	if !ok {
		consoleWarn("No such label '" + name + "' for console.timeEnd()")
		return
	}
	delete(consoleTimers, key)
	ConsoleLog(FromGoString(name + ": " + formatConsoleDuration(time.Since(started))))
}

// ConsoleTimeLog is console.timeLog, which prints how long the label's timer has
// run so far and leaves it running, so a program can mark several points inside
// one span. Anything else passed is printed after the duration.
//
// Node writes the line as log('%s: %s', label, duration, ...data), so the extra
// arguments go through the same format pass every console line does: the first
// of them can carry specifiers the ones after it fill. Handing the whole list to
// the formatter is what keeps that, rather than stringifying each on its own.
func ConsoleTimeLog(label Value, data ...Value) {
	name := consoleLabel(label)
	started, ok := consoleTimers[consoleTimerKey(label)]
	if !ok {
		consoleWarn("No such label '" + name + "' for console.timeLog()")
		return
	}
	head := []Value{
		StringValue(FromGoString("%s: %s")),
		StringValue(FromGoString(name)),
		StringValue(FromGoString(formatConsoleDuration(time.Since(started)))),
	}
	ConsoleLog(ConsoleFormat(append(head, data...)...))
}

// ConsoleGroup is console.group, which prints its arguments if it was given any
// and indents everything written after it by two spaces, until the matching
// groupEnd. Node's groupCollapsed is the same function, since a terminal has no
// way to collapse anything.
func ConsoleGroup(parts ...BStr) {
	if len(parts) > 0 {
		ConsoleLog(parts...)
	}
	consoleIndent += "  "
}

// ConsoleGroupEnd is console.groupEnd, which takes one level of indentation back
// off. At the outermost level it does nothing, rather than indent negatively.
func ConsoleGroupEnd() {
	if len(consoleIndent) >= 2 {
		consoleIndent = consoleIndent[:len(consoleIndent)-2]
	}
}

// ConsoleDir is console.dir, which writes one value's inspected form. It differs
// from console.log on a string, which log prints raw and dir prints quoted, so it
// is its own helper rather than log with one argument.
func ConsoleDir(v Value) {
	ConsoleLog(NodeInspect(v))
}

// ConsoleAssert is console.assert, which is quiet when the condition holds and
// writes "Assertion failed" to standard error when it does not. It never throws,
// which is the whole difference between it and assert(): a failed console.assert
// reports and the program carries on. The condition arrives as a value and is
// read for its truthiness, since console.assert takes whatever it is given.
//
// The message is built the way Node builds it, which takes three cases and not
// one. With nothing else passed the line is the bare prefix. With a string first,
// the prefix is glued onto it with a colon, so the string keeps its place as the
// format the arguments after it fill: console.assert(false, '%d x', 1) prints
// "Assertion failed: 1 x" rather than leaving the specifier standing. With
// anything else first, the prefix goes in front as its own argument, so the value
// is inspected the way a logged value is: console.assert(false, {a: 1}) prints
// "Assertion failed { a: 1 }", with no colon, since there is no message to
// introduce.
func ConsoleAssert(cond Value, args ...Value) {
	if ToBoolean(cond) {
		return
	}
	const prefix = "Assertion failed"
	if len(args) == 0 {
		ConsoleError(FromGoString(prefix))
		return
	}
	rest := append([]Value{}, args...)
	if rest[0].Kind() == KindString {
		rest[0] = StringValue(FromGoString(prefix + ": " + rest[0].str().ToGoString()))
	} else {
		rest = append([]Value{StringValue(FromGoString(prefix))}, rest...)
	}
	ConsoleError(ConsoleFormat(rest...))
}

// ConsoleNoop is console.timeStamp, console.profile, and console.profileEnd,
// the three members that report to an attached inspector and write nothing on
// their own. A compiled program has no inspector to report to, so there is
// nothing for them to do, and Node's own output for a program that calls them is
// the same nothing. The arguments are taken rather than dropped at the call site
// so a call still evaluates what it was passed.
func ConsoleNoop(args ...Value) {}

// consoleLabel is the label a stateful console method files its entry under: the
// string form of the argument, with an omitted one reading as "default". Node
// coerces the label rather than requiring a string, so console.count(1) and
// console.count('1') are one counter, and an object counts under its
// "[object Object]" form.
func consoleLabel(label Value) string {
	if label.Kind() == KindUndefined {
		return "default"
	}
	return ToString(label).ToGoString()
}

// consoleTimerKey is the key console.time files a timer under. Node holds the
// timers in a Map keyed by the label value itself rather than by its string
// form, so console.time(1) and console.time('1') are two timers, unlike
// console.count, which coerces its label before counting. The kind rides along
// in front of the string form here so the two stay apart, with an omitted label
// reading as the string "default" the way the parameter's own default does.
//
// A label that is an object keys by its string form rather than by identity, so
// two distinct objects share a timer where Node gives them one each. Nothing
// reaches for that; a label is a string in every program that uses one.
func consoleTimerKey(label Value) string {
	if label.Kind() == KindUndefined {
		label = StringValue(FromGoString("default"))
	}
	return strconv.Itoa(int(label.Kind())) + ":" + ToString(label).ToGoString()
}

// consoleWarn writes the warning Node's console prints when a label does not
// line up: an unknown label handed to countReset or timeEnd, or a label handed
// to time twice. Node routes these through process.emitWarning, which writes to
// standard error under a "(node:pid) Warning: " prefix, so that is the line
// written here.
//
// Node defers the write to the next tick and this one is immediate, so a warning
// lands before the synchronous output that follows it rather than after. Deferring
// would need a queue every program drains, and a program that schedules nothing
// drains nothing today, so the warning would be the thing that goes missing. The
// visible line is the same either way; only its place in the stream differs.
func consoleWarn(message string) {
	_, _ = os.Stderr.WriteString("(node:" + strconv.Itoa(os.Getpid()) + ") Warning: " + message + "\n")
}

// formatConsoleDuration renders a span the way console.timeEnd does. Under a
// second it is milliseconds to three places, under a minute it is seconds to
// three places, and past that it is the m:ss.mmm clock form with the unit spelled
// out after it, because a bare number of seconds stops being readable there.
func formatConsoleDuration(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	if ms < 1000 {
		return strconv.FormatFloat(ms, 'f', 3, 64) + "ms"
	}
	hours := 0
	minutes := 0
	if ms >= 60_000 {
		if ms >= 3_600_000 {
			hours = int(math.Floor(ms / 3_600_000))
			ms = math.Mod(ms, 3_600_000)
		}
		minutes = int(math.Floor(ms / 60_000))
		ms = math.Mod(ms, 60_000)
	}
	seconds := ms / 1000
	if hours == 0 && minutes == 0 {
		return strconv.FormatFloat(seconds, 'f', 3, 64) + "s"
	}
	whole, frac, _ := strings.Cut(strconv.FormatFloat(seconds, 'f', 3, 64), ".")
	head := strconv.Itoa(minutes)
	unit := "m:ss.mmm"
	if hours != 0 {
		head = strconv.Itoa(hours) + ":" + fmt.Sprintf("%02d", minutes)
		unit = "h:mm:ss.mmm"
	}
	return head + ":" + fmt.Sprintf("%02s", whole) + "." + frac + " (" + unit + ")"
}
