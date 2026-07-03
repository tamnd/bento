// Package structfixture is a tiny Go package the go: struct-crossing tests import.
// It exports a struct with basic fields and functions that return it, so a bento
// program can read the struct's fields back after the crossing and prove a Go struct
// result becomes a read-only object box.
package structfixture

import "strconv"

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

// Sum adds a Point's coordinates, so a struct crosses in as a parameter and the
// call reads its integer fields back on the Go side.
func Sum(p Point) int {
	return p.X + p.Y
}

// Describe renders a Profile to a string, so a struct with a string, a numeric, and
// a boolean field all cross in as one parameter.
func Describe(u Profile) string {
	suffix := "inactive"
	if u.Active {
		suffix = "active"
	}
	return u.Name + " " + strconv.Itoa(u.Age) + " " + suffix
}

// SumAll adds the coordinates of every Point, so a variadic of structs spreads each
// element through the struct crossing.
func SumAll(pts ...Point) int {
	total := 0
	for _, p := range pts {
		total += p.X + p.Y
	}
	return total
}
