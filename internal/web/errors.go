package web

import "fmt"

// DomainError ports DomainError and its subclasses (see app/errors).
type DomainError struct {
	Class   string // e.g. "InvalidDate"
	Code    string
	Message string
	Context map[string]any
}

func (e *DomainError) Error() string { return e.Message }

// InfraError ports InfrastructureError and its subclasses.
type InfraError struct {
	Class   string
	Code    string
	Message string
}

func (e *InfraError) Error() string { return e.Message }

var domainDefaults = map[string]string{
	"AuthorizationError":    "AUTHORIZATION_FAILED",
	"InvalidDate":           "INVALID_DATE",
	"InvalidParameter":      "INVALID_PARAMETER",
	"InvalidPreference":     "INVALID_PREFERENCE",
	"UnsupportedPrayerBook": "UNSUPPORTED_PRAYER_BOOK",
	"UnsupportedOfficeType": "UNSUPPORTED_OFFICE_TYPE",
	"ReadingNotFound":       "READING_NOT_FOUND",
	"InvalidBibleReference": "INVALID_BIBLE_REFERENCE",
	"DomainError":           "DOMAIN_ERROR",
}

var domainStatuses = map[string]int{
	"AuthorizationError":    403,
	"InvalidDate":           400,
	"InvalidParameter":      400,
	"InvalidPreference":     422,
	"UnsupportedPrayerBook": 404,
	"UnsupportedOfficeType": 400,
	"ReadingNotFound":       404,
	"InvalidBibleReference": 400,
}

var infraDefaults = map[string]string{
	"CacheUnavailable":           "CACHE_UNAVAILABLE",
	"ExternalServiceUnavailable": "EXTERNAL_SERVICE_UNAVAILABLE",
	"ExternalServiceRejected":    "EXTERNAL_SERVICE_REJECTED",
	"InfrastructureError":        "INFRASTRUCTURE_ERROR",
}

var infraStatuses = map[string]int{
	"CacheUnavailable":           503,
	"ExternalServiceUnavailable": 503,
	"ExternalServiceRejected":    502,
}

// NewDomainError builds a domain error; code "" uses the class default and
// message "" uses the code humanized (Rails: code.humanize).
func NewDomainError(class, message, code string) *DomainError {
	if code == "" {
		code = domainDefaults[class]
	}
	if message == "" {
		message = Humanize(code)
	}
	return &DomainError{Class: class, Code: code, Message: message}
}

// NewInfraError builds an infrastructure error.
func NewInfraError(class, message, code string) *InfraError {
	if code == "" {
		code = infraDefaults[class]
	}
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return &InfraError{Class: class, Code: code, Message: message}
}

// Status returns the HTTP status Rails maps the error to.
func (e *DomainError) Status() int {
	if s, ok := domainStatuses[e.Class]; ok {
		return s
	}
	return 422
}

func (e *InfraError) Status() int {
	if s, ok := infraStatuses[e.Class]; ok {
		return s
	}
	return 503
}

// Raise panics with a domain error, like Ruby's raise; the controller
// dispatcher rescues it the way ApplicationController's rescue_from does.
func Raise(class, message, code string) {
	panic(NewDomainError(class, message, code))
}

// StandardError is any other failure rescued by NewRelicErrorTracking.
type StandardError struct {
	Class   string
	Message string
}

func (e *StandardError) Error() string { return e.Message }

// Fail panics with a StandardError (500 in production).
func Fail(class, format string, args ...any) {
	panic(&StandardError{Class: class, Message: fmt.Sprintf(format, args...)})
}

// RecordNotFound mirrors ActiveRecord::RecordNotFound.
func RecordNotFound(message string) {
	panic(&StandardError{Class: "ActiveRecord::RecordNotFound", Message: message})
}

// Humanize mirrors ActiveSupport's String#humanize for codes like
// "INVALID_DATE" => "Invalid date".
func Humanize(s string) string {
	out := []rune{}
	for i, r := range s {
		if r == '_' {
			r = ' '
		}
		if i == 0 {
			if r >= 'a' && r <= 'z' {
				r -= 32
			}
		} else if r >= 'A' && r <= 'Z' {
			r += 32
		}
		out = append(out, r)
	}
	str := string(out)
	for len(str) > 0 && str[0] == ' ' {
		str = str[1:]
	}
	if len(str) > 0 && str[0] >= 'a' && str[0] <= 'z' {
		str = string(str[0]-32) + str[1:]
	}
	return str
}

// exceptionStatus is the status ShowExceptions answers for errors that
// escape the controller.
type exceptionStatus int

func (e exceptionStatus) Error() string { return fmt.Sprintf("status %d", int(e)) }
