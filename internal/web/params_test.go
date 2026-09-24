package web

import (
	"testing"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

func TestParseNestedQuery(t *testing.T) {
	cases := map[string]string{
		"preferences%5Bprayer_book_code%5D=loc_2015&x=1": `{"preferences":{"prayer_book_code":"loc_2015"},"x":"1"}`,
		"a[]=1&a[]=2":                `{"a":["1","2"]}`,
		"a[][b]=1&a[][c]=2&a[][b]=3": `{"a":[{"b":"1","c":"2"},{"b":"3"}]}`,
		"a=1&a=2":                    `{"a":"2"}`,
		"flag":                       `{"flag":null}`,
		"q=a+b%20c":                  `{"q":"a b c"}`,
		"x[y][z]=1":                  `{"x":{"y":{"z":"1"}}}`,
	}
	for in, want := range cases {
		got, err := ParseNestedQuery(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if s := string(rb.JSON(got)); s != want {
			t.Errorf("%s => %s want %s", in, s, want)
		}
	}
	if _, err := ParseNestedQuery("a=%zz"); err == nil {
		t.Error("expected error for malformed escape")
	}
	if _, err := ParseNestedQuery("a=1&a[b]=2"); err == nil {
		t.Error("expected type conflict error")
	}
}
