package rb

import "testing"

// Expectations from Ruby's Kernel#Integer.
func TestKernelInteger(t *testing.T) {
	ok := map[string]int64{"12": 12, " 12 ": 12, "+12": 12, "-12": -12, "0x1A": 26, "0X1a": 26, "0b101": 5, "0o17": 15,
		"017": 15, "0d19": 19, "1_000": 1000, "0_7": 7, "0": 0, "00": 0, "\t7\n": 7, "0_0": 0, "0x1_f": 31, "-0": 0}
	for in, want := range ok {
		if n, cls := KernelInteger(in); cls != "" || n != want {
			t.Errorf("Integer(%q) = %d, %s; want %d", in, n, cls, want)
		}
	}
	for _, in := range []string{"08", "1__0", "_1", "1_", "0_", "0x", "- 1", "+-1", "1e3", "12abc", "", " ", "0x_1", "0b", "1.5", "٣", "0_x1"} {
		if _, cls := KernelInteger(in); cls != "ArgumentError" {
			t.Errorf("Integer(%q) did not raise ArgumentError (%q)", in, cls)
		}
	}
	for _, in := range []any{nil, true, []any{}, NewMap()} {
		if _, cls := KernelInteger(in); cls != "TypeError" {
			t.Errorf("Integer(%v) did not raise TypeError", in)
		}
	}
	if n, cls := KernelInteger(12.9); cls != "" || n != 12 {
		t.Errorf("Integer(12.9) = %d, %s", n, cls)
	}
}
