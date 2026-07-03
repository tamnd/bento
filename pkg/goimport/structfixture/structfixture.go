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

// SumSlice adds the coordinates of every Point in a slice, so a bento array of
// objects crosses in as a Go []Point parameter and the call reads each element's
// integer fields back on the Go side. This is the argument-direction inverse of
// Diagonal's []Point result.
func SumSlice(pts []Point) int {
	total := 0
	for _, p := range pts {
		total += p.X + p.Y
	}
	return total
}

// DescribeAll renders every Profile in a slice to one comma-joined string, so a
// bento array of mixed-field objects crosses in as a Go []Profile parameter and the
// call reads a string, a numeric, and a boolean field off each element.
func DescribeAll(us []Profile) string {
	out := ""
	for i, u := range us {
		if i > 0 {
			out += ", "
		}
		out += Describe(u)
	}
	return out
}

// SumPtr adds the coordinates of a Point through a pointer, so an object box crosses
// in as a Go *Point parameter: the closure takes the address of a fresh struct and
// the call reads its integer fields back on the Go side.
func SumPtr(p *Point) int {
	return p.X + p.Y
}

// MakePointPtr builds a Point and hands back its address, so a Go *Point result
// crosses back as a read-only object box the caller reads field by field, the same
// shape as a value Point.
func MakePointPtr(x, y int) *Point {
	return &Point{X: x, Y: y}
}

// Diagonal returns the first n points on the line y=x, so a Go []Point result
// crosses back as a bento array of read-only struct boxes the caller reads element
// by element.
func Diagonal(n int) []Point {
	pts := make([]Point, n)
	for i := range pts {
		pts[i] = Point{X: i, Y: i}
	}
	return pts
}

// Profiles returns a fixed roster of Profiles, so a []struct result with mixed field
// kinds crosses back as an array whose elements carry a string, a number, and a
// boolean field each.
func Profiles() []Profile {
	return []Profile{
		{Name: "ada", Age: 36, Active: true},
		{Name: "linus", Age: 21, Active: false},
	}
}
