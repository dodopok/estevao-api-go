package rb

import "testing"

func TestJSONFloatMatchesRubyJSONGem(t *testing.T) {
	cases := map[float64]string{
		1e16: "1e+16", 1e15: "1e+15", 123456789.123: "123456789.123", 0.0001: "0.0001",
		0.00001: "0.00001", 2.5: "2.5", 100.0: "100.0", 1.5e-7: "0.00000015", 1e17: "1e+17",
		12345678901234567.0: "1.2345678901234568e+16", 3.14159: "3.14159", 1e100: "1e+100",
		9007199254740993.0: "9.007199254740992e+15", 1234567.0: "1234567.0",
		1.0: "1.0", 1e20: "1e+20",
	}
	for in, want := range cases {
		if got := JSONFloat(in); got != want {
			t.Errorf("JSONFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeEscapes(t *testing.T) {
	got := string(JSON(M("a", "x\u0001\u001f\b\f\n\t/<>& é")))
	want := `{"a":"x\u0001\u001f\b\f\n\t/<>&` + " " + `é"}`
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestJSONFloatRuntimeSum(t *testing.T) {
	a, b := 0.1, 0.2
	if got := JSONFloat(a + b); got != "0.30000000000000004" {
		t.Errorf("got %s", got)
	}
}
