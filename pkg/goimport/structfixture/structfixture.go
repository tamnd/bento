// Package structfixture is a tiny Go package the go: struct-crossing tests import.
// It exports a struct with basic fields and functions that return it, so a bento
// program can read the struct's fields back after the crossing and prove a Go struct
// result becomes a read-only object box.
package structfixture

// Point is a 2D point with exported integer coordinates.
type Point struct {
	X int
	Y int
}

// MakePoint builds a Point from two coordinates.
func MakePoint(x, y int) Point {
	return Point{X: x, Y: y}
}

// Profile is a mixed-field struct: a string name, a numeric age, and a boolean flag,
// so the crossing is exercised over every basic field kind at once.
type Profile struct {
	Name   string
	Age    int
	Active bool
}

// MakeProfile builds a Profile from its three fields.
func MakeProfile(name string, age int, active bool) Profile {
	return Profile{Name: name, Age: age, Active: active}
}
