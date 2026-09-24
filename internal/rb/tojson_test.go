package rb

import "testing"

func TestToJSONEscapesHTMLEntities(t *testing.T) {
	bs := string(rune(92))
	want := `{"a":"` + bs + `u003c` + bs + `u003e` + bs + `u0026  "}`
	if got := string(ToJSON(M("a", "<>&  "))); got != want {
		t.Fatalf("ToJSON = %s, want %s", got, want)
	}
}
