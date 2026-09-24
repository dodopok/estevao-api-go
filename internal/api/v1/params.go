package v1

import (
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// requirePermit ports
//
//	params.require(key).permit(*fields)
//	rescue ActionController::ParameterMissing
//	  params.permit(*fields)
//
// require returns any present value (or false); a value that is not a Hash
// then fails on #permit with NoMethodError.
func requirePermit(c *web.Context, key string, fields ...string) *rb.Map {
	src := c.Params()
	if v, ok := src.Lookup(key); ok && (rb.Present(v) || v == false) {
		m, isMap := v.(*rb.Map)
		if !isMap {
			panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage("permit", v)})
		}
		src = m
	}
	out := rb.NewMap()
	for _, k := range fields {
		if v, ok := src.Lookup(k); ok && permittedScalar(v) {
			out.Set(k, v)
		}
	}
	return out
}

// paramsDig ports params.dig(a, b) on decoded parameters.
func paramsDig(c *web.Context, a, b string) any {
	v := c.Params().Get(a)
	switch x := v.(type) {
	case nil:
		return nil
	case *rb.Map:
		return x.Get(b)
	case []any:
		panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of Symbol into Integer"})
	}
	panic(&web.StandardError{Class: "TypeError", Message: rb.ClassName(v) + " does not have #dig method"})
}

// dateParseParam ports Date.parse(value) on a parameter, with
// `rescue Date::Error` turning into ArgumentError(message) (other
// ArgumentErrors, like the length limit, keep their own message).
func dateParseParam(v any, message string) rb.YMD {
	s, ok := v.(string)
	if !ok {
		desc := rb.ClassName(v)
		if _, isMap := v.(*rb.Map); isMap {
			desc = "ActionController::Parameters"
		}
		switch x := v.(type) {
		case nil:
			desc = "nil"
		case bool:
			desc = "false"
			if x {
				desc = "true"
			}
		}
		panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of " + desc + " into String"})
	}
	d, err := rb.DateParse(s, true)
	if err != nil {
		if err == rb.ErrInvalidDate {
			raiseArgument(message)
		}
		raiseArgument(err.Error())
	}
	return d
}
