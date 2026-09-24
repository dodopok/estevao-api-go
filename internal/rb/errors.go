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
