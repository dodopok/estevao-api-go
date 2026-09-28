package v2

import (
	"encoding/base64"
	"sort"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/civil"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// domainError raises a DomainError with an ordered context (Rails'
// `raise Klass.new(message, code:, context:)`).
func domainError(class, message, code string, context *rb.Map) {
	e := web.NewDomainError(class, message, code)
	e.Context = context
	panic(e)
}

// toS ports #to_s on a request parameter (Array and Hash print as their
// inspect, as ActionController::Parameters#to_s delegates to its hash).
func toS(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case *rb.Map, []any:
		return paramsToS(x)
	}
	return rb.ToS(v)
}

func paramsToS(v any) string {
	switch x := v.(type) {
	case *rb.Map:
		var parts []string
		x.Each(func(k string, e any) { parts = append(parts, rb.InspectString(k)+"=>"+paramsInspect(e)) })
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		return paramsInspect(x)
	}
	return rb.ToS(v)
}

func paramsInspect(v any) string {
	switch x := v.(type) {
	case *rb.Map:
		return "#<ActionController::Parameters " + paramsToS(x) + " permitted: false>"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = paramsInspect(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return rb.Inspect(v)
}

// presence ports Object#presence on a parameter: nil when blank.
func presence(v any) any {
	if rb.Blank(v) {
		return nil
	}
	return v
}

func strs(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// --- IncludeSet ---------------------------------------------------------------

// IncludeSet ports Api::V2::IncludeSet.
type IncludeSet struct{ names []string }

// ParseIncludes ports IncludeSet.parse.
func ParseIncludes(raw any, allowed []string) *IncludeSet {
	var names []string
	for _, n := range strings.Split(toS(raw), ",") {
		if n = rb.Strip(n); !rb.BlankString(n) {
			names = append(names, n)
		}
	}
	allowedSet := map[string]bool{}
	for _, a := range allowed {
		allowedSet[a] = true
	}
	var unknown []string
	for _, n := range names {
		if !allowedSet[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		domainError("InvalidParameter", "Include inválido: "+strings.Join(unknown, ", ")+".", "INVALID_INCLUDE",
			rb.M("unknown", strs(unknown), "allowed", strs(allowed)))
	}
	seen := map[string]bool{}
	var expanded []string
	for _, n := range names {
		parts := strings.Split(n, ".")
		for depth := 1; depth <= len(parts); depth++ {
			a := strings.Join(parts[:depth], ".")
			if !seen[a] {
				seen[a] = true
				if allowedSet[a] {
					expanded = append(expanded, a)
				}
			}
		}
	}
	return NewIncludeSet(expanded)
}

// NewIncludeSet ports IncludeSet.new (names sorted).
func NewIncludeSet(names []string) *IncludeSet {
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	return &IncludeSet{names: sorted}
}

// Has ports include?.
func (s *IncludeSet) Has(name string) bool {
	for _, n := range s.names {
		if n == name {
			return true
		}
	}
	return false
}

// Names ports to_a.
func (s *IncludeSet) Names() []string { return append([]string{}, s.names...) }

// CacheKey ports cache_key.
func (s *IncludeSet) CacheKey() string {
	if len(s.names) == 0 {
		return "none"
	}
	return strings.Join(s.names, ",")
}

// --- FieldSet ---------------------------------------------------------------

// FieldSet ports Api::V2::FieldSet: per type, the selected attributes.
type FieldSet struct {
	types      []string
	selections map[string][]string
}

// fieldSpec is ALLOWED_FIELDS: type name -> attributes, in declaration order.
type fieldSpec struct {
	types  []string
	fields map[string][]string
}

func spec(pairs ...any) fieldSpec {
	s := fieldSpec{fields: map[string][]string{}}
	for i := 0; i < len(pairs); i += 2 {
		t := pairs[i].(string)
		s.types = append(s.types, t)
		s.fields[t] = pairs[i+1].([]string)
	}
	return s
}

func invalidFields(message string, context *rb.Map) {
	if context == nil {
		context = rb.NewMap()
	}
	domainError("InvalidParameter", message, "INVALID_FIELDS", context)
}

// ParseFields ports FieldSet.parse.
func ParseFields(raw any, allowed fieldSpec) *FieldSet {
	fs := &FieldSet{selections: map[string][]string{}}
	if rb.Blank(raw) {
		return fs
	}
	m, ok := raw.(*rb.Map)
	if !ok {
		invalidFields("fields deve ser informado como fields[tipo]=campo,campo", nil)
	}
	m.Each(func(typ string, names any) {
		known, ok := allowed.fields[typ]
		if !ok {
			invalidFields("Tipo desconhecido em fields: '"+typ+"'.", rb.M("allowed", strs(allowed.types)))
		}
		var fields, unknown []string
		for _, n := range strings.Split(toS(names), ",") {
			if n = rb.Strip(n); rb.BlankString(n) {
				continue
			}
			fields = append(fields, n)
		}
		for _, f := range fields {
			found := false
			for _, k := range known {
				if k == f {
					found = true
				}
			}
			if !found {
				unknown = append(unknown, f)
			}
		}
		if len(unknown) > 0 {
			invalidFields("Campos desconhecidos em fields["+typ+"]: "+strings.Join(unknown, ", ")+".",
				rb.M("type", typ, "allowed", strs(known)))
		}
		if _, dup := fs.selections[typ]; !dup {
			fs.types = append(fs.types, typ)
		}
		fs.selections[typ] = fields
	})
	return fs
}

// Apply ports FieldSet#apply: the attributes selected for type, in the
// order they were named; a type nobody trimmed is returned whole.
func (f *FieldSet) Apply(typ string, attributes *rb.Map) *rb.Map {
	selected := f.selections[typ]
	if len(selected) == 0 {
		return attributes
	}
	out := rb.NewMap()
	for _, k := range selected {
		if v, ok := attributes.Lookup(k); ok {
			out.Set(k, v)
		}
	}
	return out
}

// Any ports any?.
func (f *FieldSet) Any() bool { return len(f.types) > 0 }

// CacheKey ports cache_key.
func (f *FieldSet) CacheKey() string {
	if len(f.types) == 0 {
		return "none"
	}
	types := append([]string{}, f.types...)
	sort.Strings(types)
	parts := make([]string, len(types))
	for i, t := range types {
		names := append([]string{}, f.selections[t]...)
		sort.Strings(names)
		parts[i] = t + ":" + strings.Join(names, ",")
	}
	return strings.Join(parts, ";")
}

// --- RequestCost ------------------------------------------------------------

var costWeights = map[string]int{
	"readings": 1, "readings.text": 3, "readings.morning": 1, "readings.evening": 1,
	"readings.alternatives": 0, "collect": 1, "collect.text": 0, "celebrations": 0, "explanation": 4,
}

// RequestCost ports RequestCost.for.
func RequestCost(includes *IncludeSet, days int) int {
	perDay := 1
	for _, n := range includes.names {
		if w, ok := costWeights[n]; ok {
			perDay += w
		} else {
			perDay++
		}
	}
	if days < 1 {
		days = 1
	}
	return perDay * days
}

// --- Dates ----------------------------------------------------------------

const (
	maxRangeDays                = 366
	maxRangeDaysWithText        = 31
	maxRangeDaysWithExplanation = 31
)

func isoDay(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// ParseDate ports Dates.parse.
func ParseDate(value any) civil.Date {
	s := toS(value)
	if s == "today" {
		return today()
	}
	if isoDay(s) {
		if ymd, err := rb.DateISO8601(s); err == nil {
			if d, ok := civil.New(int(ymd.Y), int(ymd.M), int(ymd.D)); ok {
				return d
			}
		}
	}
	domainError("InvalidDate", "Data inválida: '"+s+"'. Use o formato AAAA-MM-DD ou 'today'.", "", rb.M("date", s))
	return 0
}

// YearBounds ports Dates.year_bounds.
func YearBounds(year any) (civil.Date, civil.Date) {
	value := toS(year)
	n := rb.StringToI(value)
	ok := len(value) == 4
	for _, c := range value {
		if c < '0' || c > '9' {
			ok = false
		}
	}
	if !ok || n < 1900 || n > 2200 {
		domainError("InvalidDate", "Ano deve estar entre 1900 e 2200", "", rb.M("year", value))
	}
	return civil.MustNew(n, 1, 1), civil.MustNew(n, 12, 31)
}

// RangeLimit ports Dates.range_limit.
func RangeLimit(includes *IncludeSet) int {
	limit := maxRangeDays
	if includes.Has("readings.text") && maxRangeDaysWithText < limit {
		limit = maxRangeDaysWithText
	}
	if includes.Has("explanation") && maxRangeDaysWithExplanation < limit {
		limit = maxRangeDaysWithExplanation
	}
	return limit
}

// DateRange ports Dates.range.
func DateRange(c *web.Context, limit int) (civil.Date, civil.Date) {
	var first, last civil.Date
	if c.ParamPresent("year") {
		first, last = YearBounds(c.Param("year"))
	} else {
		if !c.ParamPresent("from") || !c.ParamPresent("to") {
			domainError("InvalidParameter", "Informe 'from' e 'to' (ISO 8601) ou 'year'.", "MISSING_DATE_RANGE", nil)
		}
		first, last = ParseDate(c.Param("from")), ParseDate(c.Param("to"))
	}
	if last < first {
		domainError("InvalidParameter", "'to' não pode ser anterior a 'from'.", "INVALID_DATE_RANGE",
			rb.M("from", first.ISO(), "to", last.ISO()))
	}
	days := last.Sub(first) + 1
	if days > limit {
		domainError("InvalidParameter", "Intervalo de "+strconv.Itoa(days)+" dias excede o máximo de "+strconv.Itoa(limit)+" para esta consulta.",
			"RANGE_TOO_LARGE", rb.M("days", days, "max_days", limit))
	}
	return first, last
}

// --- CollectionCursor -------------------------------------------------------

const (
	defaultLimit = 50
	maxLimit     = 100
)

// CursorLimit ports CollectionCursor.limit.
func CursorLimit(value any) int {
	if rb.Blank(value) {
		return defaultLimit
	}
	raw := toS(value)
	n := rb.StringToI(raw)
	digits := raw != ""
	for _, c := range raw {
		if c < '0' || c > '9' {
			digits = false
		}
	}
	if digits && n >= 1 && n <= maxLimit {
		return n
	}
	domainError("InvalidParameter", "limit deve ser um número entre 1 e "+strconv.Itoa(maxLimit)+".", "INVALID_LIMIT",
		rb.M("limit", raw, "max", maxLimit))
	return 0
}

func invalidCursor(message string) {
	domainError("InvalidParameter", message, "INVALID_CURSOR", nil)
}

// urlsafeDecode64 ports Base64.urlsafe_decode64 (padding added when the
// string does not end with '=', then strict decoding).
func urlsafeDecode64(s string) ([]byte, bool) {
	if !strings.HasSuffix(s, "=") && len(s)%4 != 0 {
		s += strings.Repeat("=", (len(s)+3)&^3-len(s))
	}
	s = strings.NewReplacer("-", "+", "_", "/").Replace(s)
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	return b, err == nil
}

// DecodeCursor ports CollectionCursor.decode (nil when blank).
func DecodeCursor(value any, scope string) []any {
	if rb.Blank(value) {
		return nil
	}
	raw, ok := urlsafeDecode64(toS(value))
	if !ok {
		invalidCursor("Cursor inválido.")
	}
	parsed, err := rb.ParseJSON(raw)
	if err != nil {
		invalidCursor("Cursor inválido.")
	}
	m, isMap := parsed.(*rb.Map)
	if isMap {
		key, isArray := m.Get("key").([]any)
		if numericEqual(m.Get("version"), 1) && m.Get("scope") == scope && isArray {
			if key == nil {
				key = []any{}
			}
			return key
		}
	}
	invalidCursor("Cursor inválido para esta consulta.")
	return nil
}

func numericEqual(v any, n int) bool {
	switch x := v.(type) {
	case int:
		return x == n
	case int64:
		return x == int64(n)
	case float64:
		return x == float64(n)
	}
	return false
}

// EncodeCursor ports CollectionCursor.encode.
func EncodeCursor(key []any, scope string) string {
	payload := rb.JSONGenerate(rb.M("version", 1, "scope", scope, "key", key))
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// compare ports <=> between JSON-shaped values (nil when incomparable).
func compare(a, b any) (int, bool) {
	if af, ok := number(a); ok {
		bf, ok := number(b)
		if !ok {
			return 0, false
		}
		switch {
		case af < bf:
			return -1, true
		case af > bf:
			return 1, true
		}
		return 0, true
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		if !ok {
			return 0, false
		}
		return strings.Compare(x, y), true
	case []any:
		y, ok := b.([]any)
		if !ok {
			return 0, false
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			c, ok := compare(x[i], y[i])
			if !ok {
				return 0, false
			}
			if c != 0 {
				return c, true
			}
		}
		switch {
		case len(x) < len(y):
			return -1, true
		case len(x) > len(y):
			return 1, true
		}
		return 0, true
	}
	// Object#<=>: 0 when equal, otherwise incomparable.
	if rubyEqual(a, b) {
		return 0, true
	}
	return 0, false
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func rubyEqual(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case *rb.Map:
		y, ok := b.(*rb.Map)
		if !ok || x.Len() != y.Len() {
			return false
		}
		eq := true
		x.Each(func(k string, v any) {
			w, ok := y.Lookup(k)
			if !ok {
				eq = false
				return
			}
			if c, ok := compare(v, w); !ok || c != 0 {
				if !rubyEqual(v, w) {
					eq = false
				}
			}
		})
		return eq
	}
	c, ok := compare(a, b)
	return ok && c == 0
}

// CursorPage ports CollectionCursor.page.
func CursorPage(items []*rb.Map, limit int, cursor []any, hasCursor bool, scope string, key func(*rb.Map) []any) ([]*rb.Map, string) {
	ordered := append([]*rb.Map{}, items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		c, _ := compare(key(ordered[i]), key(ordered[j]))
		return c < 0
	})
	if hasCursor {
		i := 0
		for ; i < len(ordered); i++ {
			c, ok := compare(key(ordered[i]), cursor)
			if !ok {
				invalidCursor("Cursor inválido.")
			}
			if c > 0 {
				break
			}
		}
		ordered = ordered[i:]
	}
	page := ordered
	if len(page) > limit {
		page = page[:limit]
	}
	next := ""
	if len(ordered) > limit && len(page) > 0 {
		next = EncodeCursor(key(page[len(page)-1]), scope)
	}
	return page, next
}
