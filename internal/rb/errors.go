package rb

// RubyError is an exception Ruby would raise from core methods (KeyError,
// NoMethodError on nil, ...). Ported code panics with it where the Rails
// code would crash, so the request fails the same way (500 with the same
// message in production).
type RubyError struct {
	Class   string
	Message string
}

func (e *RubyError) Error() string { return e.Message }

// RaiseNoMethodOnNil mirrors calling +method+ on nil (Ruby 3.2 wording).
func RaiseNoMethodOnNil(method string) {
	panic(&RubyError{Class: "NoMethodError", Message: "undefined method `" + method + "' for nil:NilClass"})
}

// RaiseKeyError mirrors Hash#fetch on a missing key; key is its #inspect.
func RaiseKeyError(inspectedKey string) {
	panic(&RubyError{Class: "KeyError", Message: "key not found: " + inspectedKey})
}

// NoMethodErrorMessage renders Ruby 3.2's NoMethodError message for a
// receiver: its inspect (when at most 65 characters) and class.
func NoMethodErrorMessage(method string, recv any) string {
	if recv == nil {
		return "undefined method `" + method + "' for nil:NilClass"
	}
	desc := Inspect(recv)
	if len([]rune(desc)) > 65 {
		desc = "#<" + ClassName(recv) + ">"
	}
	return "undefined method `" + method + "' for " + desc + ":" + ClassName(recv)
}
