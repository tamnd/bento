package lower

import (
	"strings"
	"testing"
)

// TestArrayFromAsyncEmits pins the shape: fromAsync over a sync array mints a
// value.RunAsync coroutine that awaits each element and collects it, awaiting a
// promise element to its value and a plain element to itself.
func TestArrayFromAsyncEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"promises",
			"async function run(p: Promise<number>[]): Promise<number[]> { return await Array.fromAsync(p); }\nrun([Promise.resolve(1)]);\n",
			"value.Await(",
		},
		{
			"plain-values",
			"async function run(a: number[]): Promise<number[]> { return await Array.fromAsync(a); }\nrun([1]);\n",
			"value.AwaitValue[float64](",
		},
		{
			"runs-async",
			"async function run(a: number[]): Promise<number[]> { return await Array.fromAsync(a); }\nrun([1]);\n",
			"value.RunAsync[*value.Array[float64]](",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("Array.fromAsync did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestArrayFromAsyncRuns checks the collected array end to end: fromAsync over an
// array of promises awaits each to its fulfilled value, and over an array of plain
// values awaits each to itself, so the sum reads the fulfilled elements.
func TestArrayFromAsyncRuns(t *testing.T) {
	src := `
async function run(): Promise<void> {
  const proms = [Promise.resolve(1), Promise.resolve(2)];
  const a = await Array.fromAsync(proms);
  const b = await Array.fromAsync([10, 20]);
  console.log("" + (a[0] + a[1] + b[0] + b[1]));
}
run();
`
	got := runProgramGo(t, src)
	if got != "33\n" {
		t.Fatalf("Array.fromAsync sum = %q, want %q", got, "33\n")
	}
}
