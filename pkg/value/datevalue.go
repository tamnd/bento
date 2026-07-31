// This file bridges a *Date into the dynamic value.Value world: the box a date takes
// when it flows into an any binding, a console.log argument, an assert call, or a
// JSON.stringify. It is the Date reading of the same seam mapvalue.go opens for a Map,
// a brand field on Object plus a member switch, and it reads the same way.
//
// The box is a view, not a copy. An ordinary object value carries the live date in its
// jsDate field, so typeof reports "object", the value is truthy, and a write through
// the box reaches the same instant the typed side holds: a boxed d.setFullYear(2000) is
// visible to the typed d.getFullYear() that follows it. The date caches its own box, so
// boxing one date twice yields one object and `a === b` holds for two bindings of the
// same date, which a fresh box per crossing would get wrong.
//
// A Date has no own properties in JavaScript, which is why the member surface is a
// switch here rather than properties installed on the box: Object.keys of a date is
// empty in Node and stays empty here, while a read of .getTime finds the live instant.
// The one thing a date carries that a collection does not is a Symbol.toPrimitive of
// its own, and it is the reason a date is the one built-in that concatenates under +
// rather than adding: its hook answers the string form for the default hint.

package value

import "math"

// ToValue boxes a date into a dynamic value. The box is built once and kept on the
// date, so every crossing of the same date hands back the same object: a JavaScript
// Date is a reference, and two boxes would compare unequal under === and print as two
// values under console.log even though the program has one date.
func (d *Date) ToValue() Value {
	if d.boxed == nil {
		d.boxed = &Object{kind: KindObject, jsDate: d}
	}
	return objectValue(d.boxed)
}

// asDate returns the live date a value boxes, or nil when the value is not a date box.
// It is the probe the reads, the inspector, the class tag and the deep comparison make
// before their ordinary object handling, the same shape asMap and asRegExp have.
func (v Value) asDate() *Date {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsDate
}

// dateNumberReads are the no-argument methods that answer a number: the time value and
// every calendar component, local and UTC. Each is a method expression rather than a
// closure, so the table is the same list the statically-typed path lowers to and the
// two cannot drift into answering different things for the same name.
var dateNumberReads = map[string]func(*Date) float64{
	"getTime":            (*Date).GetTime,
	"valueOf":            (*Date).ValueOf,
	"getTimezoneOffset":  (*Date).GetTimezoneOffset,
	"getFullYear":        (*Date).GetFullYear,
	"getMonth":           (*Date).GetMonth,
	"getDate":            (*Date).GetDate,
	"getDay":             (*Date).GetDay,
	"getHours":           (*Date).GetHours,
	"getMinutes":         (*Date).GetMinutes,
	"getSeconds":         (*Date).GetSeconds,
	"getMilliseconds":    (*Date).GetMilliseconds,
	"getUTCFullYear":     (*Date).GetUTCFullYear,
	"getUTCMonth":        (*Date).GetUTCMonth,
	"getUTCDate":         (*Date).GetUTCDate,
	"getUTCDay":          (*Date).GetUTCDay,
	"getUTCHours":        (*Date).GetUTCHours,
	"getUTCMinutes":      (*Date).GetUTCMinutes,
	"getUTCSeconds":      (*Date).GetUTCSeconds,
	"getUTCMilliseconds": (*Date).GetUTCMilliseconds,
}

// dateTextReads are the no-argument methods that answer a string: the five formats. The
// missing sixth, toJSON, answers null for an unrepresentable instant rather than text,
// so it is not one of these and is spelled out in the switch below.
var dateTextReads = map[string]func(*Date) BStr{
	"toISOString":  (*Date).ToISOString,
	"toString":     (*Date).ToString,
	"toDateString": (*Date).ToDateString,
	"toTimeString": (*Date).ToTimeString,
	"toUTCString":  (*Date).ToUTCString,
}

// dateComponentWrites are the calendar setters, each taking its own field and,
// optionally, the fields below it. Every one gives back the new time value, which is
// what makes a boxed d.setDate(1) usable as an expression the way the typed one is.
var dateComponentWrites = map[string]func(*Date, ...float64) float64{
	"setFullYear":        (*Date).SetFullYear,
	"setMonth":           (*Date).SetMonth,
	"setDate":            (*Date).SetDate,
	"setHours":           (*Date).SetHours,
	"setMinutes":         (*Date).SetMinutes,
	"setSeconds":         (*Date).SetSeconds,
	"setMilliseconds":    (*Date).SetMilliseconds,
	"setUTCFullYear":     (*Date).SetUTCFullYear,
	"setUTCMonth":        (*Date).SetUTCMonth,
	"setUTCDate":         (*Date).SetUTCDate,
	"setUTCHours":        (*Date).SetUTCHours,
	"setUTCMinutes":      (*Date).SetUTCMinutes,
	"setUTCSeconds":      (*Date).SetUTCSeconds,
	"setUTCMilliseconds": (*Date).SetUTCMilliseconds,
}

// dateGet reads a member off a boxed date, mirroring the concrete methods the
// statically-typed path emits: each name reads a callable bound to the live date, so a
// dynamic d.setTime(0) lands in the same instant the typed d.getTime() reads. A name
// that is not a Date member reports ok=false, so the caller climbs the ordinary chain
// and answers undefined for a miss, which is what a read of an unrelated name off a
// date does in JavaScript.
func dateGet(d *Date, name string) (Value, bool) {
	if read, ok := dateNumberReads[name]; ok {
		return boundMethod(name, func(args []Value) Value { return Number(read(d)) }), true
	}
	if read, ok := dateTextReads[name]; ok {
		return boundMethod(name, func(args []Value) Value { return StringValue(read(d)) }), true
	}
	if write, ok := dateComponentWrites[name]; ok {
		return boundMethod(name, func(args []Value) Value { return Number(write(d, dateWriteArgs(args)...)) }), true
	}
	switch name {
	case "setTime":
		// setTime is the one write that takes an instant rather than a calendar field, so
		// it takes a single number and does not go through the component rebuild.
		return boundMethod("setTime", func(args []Value) Value {
			return Number(d.SetTime(ToNumber(Arg(args, 0))))
		}), true
	case "toJSON":
		// toJSON answers the value null for an unrepresentable instant, which is the whole
		// reason JSON.stringify of an invalid date writes null rather than throwing the way
		// toISOString does.
		return boundMethod("toJSON", func(args []Value) Value { return d.ToJSON() }), true
	}
	return Undefined, false
}

// dateWriteArgs coerces a boxed setter's arguments into the numbers the component
// setters take. A call with no argument at all still writes one component: the
// specification reads the first field as ToNumber(undefined), which is NaN, so
// date.setMonth() with nothing to set makes the date invalid rather than leaving it
// where it was.
func dateWriteArgs(args []Value) []float64 {
	if len(args) == 0 {
		return []float64{math.NaN()}
	}
	out := make([]float64, len(args))
	for i, a := range args {
		out[i] = ToNumber(a)
	}
	return out
}

// dateSymGet reads a symbol-keyed member off a boxed date. There is one, and it is the
// reason a date behaves unlike every other object under +: Date.prototype's
// Symbol.toPrimitive answers the string form for the default hint, where the ordinary
// coercion would take valueOf and give a number. So "" + d and `${d}` and d + 1 all
// read the local time, while d - 0 and d < e read the instant.
//
// A Date carries no Symbol.toStringTag: Object.prototype.toString names it through its
// internal slot, which ClassTag reads off the brand, so answering a tag here would put
// a property in a key walk that Node leaves empty.
func dateSymGet(d *Date, key *Symbol) (Value, bool) {
	if key != symbolToPrimitive {
		return Undefined, false
	}
	return boundMethod("[Symbol.toPrimitive]", func(args []Value) Value {
		hint := Arg(args, 0)
		if hint.kind == KindString {
			switch hint.str().ToGoString() {
			case "number":
				return Number(d.ValueOf())
			case "string", "default":
				return StringValue(d.ToString())
			}
		}
		// The hook is a method a program can call itself, and the specification has it
		// reject anything but the three hints rather than pick one, so a mistyped hint is
		// an error at the call rather than a silently different coercion.
		Throw(NewTypeError(FromGoString("Invalid hint: " + ToString(hint).ToGoString())))
		return Undefined
	}), true
}

// dateInspectText is how console.log and util.inspect spell a date: the ISO form, which
// names the instant unambiguously, and the human "Invalid Date" for a date that names
// no instant at all, since there is no ISO spelling of one and toISOString throws rather
// than inventing one.
func dateInspectText(d *Date) string {
	if math.IsNaN(d.GetTime()) {
		return d.ToString().ToGoString()
	}
	return d.ToISOString().ToGoString()
}

// dateSameInstant reports whether two dates name the same moment, the comparison
// assert.deepStrictEqual makes on a pair of them. Two Invalid Dates are the same date
// here even though their time values are both NaN and NaN is equal to nothing: node
// compares them as values rather than as numbers, so two dates that failed to parse
// are deeply equal, and a test that builds one from bad input can say so.
func dateSameInstant(a, b *Date) bool {
	x, y := a.GetTime(), b.GetTime()
	return x == y || (math.IsNaN(x) && math.IsNaN(y))
}
