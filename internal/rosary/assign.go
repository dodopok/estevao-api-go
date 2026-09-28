package rosary

import (
	"github.com/dodopok/estevao-api-go/internal/ar"
	"regexp"
	"strconv"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// ParameterMissing is ActionController::ParameterMissing.
type ParameterMissing struct{ Param string }

func (e *ParameterMissing) Error() string {
	return "param is missing or the value is empty: " + e.Param
}

// NotFound is ActiveRecord::RecordNotFound (rendered as {"error":"not_found"}).
type NotFound struct{ Message string }

func (e *NotFound) Error() string { return e.Message }

var (
	prayerFields = []string{"client_id", "title", "description", "locale", "cycle_repeat", "is_public"}
	blockFields  = []string{"id", "client_id", "position", "name", "repeat_count", "in_cycle", "_destroy"}
	stepFields   = []string{"id", "client_id", "position", "step_type", "title", "text", "repeat_count", "_destroy"}
	digitKey     = regexp.MustCompile(`\A-?[0-9]+\z`)
)

func noMethod(method string, recv any) {
	panic(&web.StandardError{Class: "NoMethodError", Message: rb.NoMethodErrorMessage(method, recv)})
}

func scalar(v any) bool {
	switch v.(type) {
	case *rb.Map, []any:
		return false
	}
	return true
}

// Permit ports
//
//	params.require(:custom_rosary_prayer).permit(:client_id, ..., blocks_attributes: [..., { steps_attributes: [...] }])
func Permit(params *rb.Map) *rb.Map {
	v, ok := params.Lookup("custom_rosary_prayer")
	if !ok || !(rb.Present(v) || v == false) {
		panic(&ParameterMissing{Param: "custom_rosary_prayer"})
	}
	m, isMap := v.(*rb.Map)
	if !isMap {
		noMethod("permit", v)
	}
	return permitHash(m, prayerFields, "blocks_attributes", permitBlock)
}

func permitBlock(m *rb.Map) *rb.Map {
	return permitHash(m, blockFields, "steps_attributes", permitStep)
}

func permitStep(m *rb.Map) *rb.Map { return permitHash(m, stepFields, "", nil) }

func permitHash(m *rb.Map, fields []string, nestedKey string, nested func(*rb.Map) *rb.Map) *rb.Map {
	out := rb.NewMap()
	for _, f := range fields {
		if v, ok := m.Lookup(f); ok && scalar(v) {
			out.Set(f, v)
		}
	}
	if nestedKey != "" {
		if v, ok := m.Lookup(nestedKey); ok {
			if permitted, ok := permitNested(v, nested); ok {
				out.Set(nestedKey, permitted)
			}
		}
	}
	return out
}

// permitNested ports permit_hash_or_array for a nested filter.
func permitNested(v any, permit func(*rb.Map) *rb.Map) (any, bool) {
	return web.PermitHashOrArray(v, permit)
}

// --- nested attributes -----------------------------------------------------

func destroyFlag(attrs *rb.Map) bool { return ar.Truthy(ar.CastBool(attrs.Get("_destroy"))) }

// collectionFor ports the Hash/Array handling of
// assign_nested_attributes_for_collection_association.
func collectionFor(v any, loaded bool) []any {
	switch x := v.(type) {
	case []any:
		return x
	case *rb.Map:
		if x.Has("id") {
			return []any{x}
		}
		var out []any
		x.Each(func(_ string, v any) { out = append(out, v) })
		if !loaded {
			// attribute_ids = collection.filter_map { |a| a["id"] }
			for _, el := range out {
				switch e := el.(type) {
				case int, int64, []any:
					panic(&web.StandardError{Class: "TypeError", Message: "no implicit conversion of String into Integer"})
				case float64, bool, nil:
					noMethod("[]", e)
				}
			}
		}
		return out
	}
	panic(&web.StandardError{Class: "ArgumentError", Message: "Hash or Array expected for attributes, got " + rb.ClassName(v)})
}

func attrsOf(el any) *rb.Map {
	m, ok := el.(*rb.Map)
	if !ok {
		noMethod("with_indifferent_access", el)
	}
	return m
}

// idToS is `id.to_s` of a nested attributes id.
func idToS(v any) string { return rb.ToS(v) }

func notFound(model string, id any, owner string, ownerID int64, ownerNew bool) {
	oid := strconv.FormatInt(ownerID, 10)
	if ownerNew {
		oid = ""
	}
	panic(&NotFound{Message: "Couldn't find " + model + " with ID=" + idToS(id) + " for " + owner + " with ID=" + oid})
}

// assignPrayer ports StructureAssignment#call's assign_attributes.
func (p *Prayer) assignAttributes(attrs *rb.Map) {
	var nested any
	hasNested := false
	attrs.Each(func(k string, v any) {
		if k == "blocks_attributes" {
			if _, isMap := v.(*rb.Map); isMap {
				nested, hasNested = v, true // hashes are assigned after the plain attributes
				return
			}
			p.assignBlocks(v)
			return
		}
		p.Record.Assign(k, v)
	})
	if hasNested {
		p.assignBlocks(nested)
	}
}

func (p *Prayer) assignBlocks(v any) {
	loaded := !p.NewRecord
	for _, el := range collectionFor(v, loaded) {
		attrs := attrsOf(el)
		id := attrs.Get("id")
		if rb.Blank(id) {
			if !destroyFlag(attrs) {
				b := &Block{Record: ar.NewRecord(blockSchema)}
				b.assignAttributes(attrs)
				p.Blocks = append(p.Blocks, b)
			}
			continue
		}
		var found *Block
		if loaded {
			for _, b := range p.Blocks {
				if !b.NewRecord && strconv.FormatInt(b.ID, 10) == idToS(id) {
					found = b
					break
				}
			}
		}
		if found == nil {
			notFound("CustomRosaryBlock", id, "CustomRosaryPrayer", p.ID, p.NewRecord)
		}
		found.assignAttributes(attrs)
		if destroyFlag(attrs) {
			found.Marked = true
		}
	}
}

func (b *Block) assignAttributes(attrs *rb.Map) {
	var nested any
	hasNested := false
	attrs.Each(func(k string, v any) {
		switch k {
		case "id", "_destroy":
		case "steps_attributes":
			if _, isMap := v.(*rb.Map); isMap {
				nested, hasNested = v, true
				return
			}
			b.assignSteps(v)
		default:
			b.Assign(k, v)
		}
	})
	if hasNested {
		b.assignSteps(nested)
	}
}

func (b *Block) assignSteps(v any) {
	loaded := !b.NewRecord
	for _, el := range collectionFor(v, loaded) {
		attrs := attrsOf(el)
		id := attrs.Get("id")
		if rb.Blank(id) {
			if !destroyFlag(attrs) {
				s := &Step{ar.NewRecord(stepSchema)}
				s.assignAttributes(attrs)
				b.Steps = append(b.Steps, s)
			}
			continue
		}
		var found *Step
		if loaded {
			for _, s := range b.Steps {
				if !s.NewRecord && strconv.FormatInt(s.ID, 10) == idToS(id) {
					found = s
					break
				}
			}
		}
		if found == nil {
			notFound("CustomRosaryStep", id, "CustomRosaryBlock", b.ID, b.NewRecord)
		}
		found.assignAttributes(attrs)
		if destroyFlag(attrs) {
			found.Marked = true
		}
	}
}

func (s *Step) assignAttributes(attrs *rb.Map) {
	attrs.Each(func(k string, v any) {
		if k != "id" && k != "_destroy" {
			s.Assign(k, v)
		}
	})
}

// --- StructureAssignment pruning ------------------------------------------

func payloadCollection(v any) []*rb.Map {
	if rb.Blank(v) {
		return nil
	}
	var items []any
	switch x := v.(type) {
	case *rb.Map:
		x.Each(func(_ string, v any) { items = append(items, v) })
	case []any:
		items = x
	default:
		noMethod("map", v)
	}
	out := make([]*rb.Map, len(items))
	for i, el := range items {
		out[i] = attrsOf(el)
	}
	return out
}

// rubyToI is `value.to_i` on a payload id.
func rubyToI(v any) int64 {
	switch x := v.(type) {
	case nil:
		return 0
	case string:
		return int64(rb.StringToI(x))
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	}
	noMethod("to_i", v)
	return 0
}

func keptIDs(payloads []*rb.Map) map[int64]bool {
	kept := map[int64]bool{}
	for _, item := range payloads {
		if destroyFlag(item) {
			continue
		}
		if id := item.Get("id"); rb.Present(id) {
			kept[rubyToI(id)] = true
		}
	}
	return kept
}

// ApplyStructure ports CustomRosaryPrayers::StructureAssignment.call.
func (p *Prayer) ApplyStructure(attrs *rb.Map) {
	p.assignAttributes(attrs)
	raw, ok := attrs.Lookup("blocks_attributes")
	if !ok {
		return
	}
	payloads := payloadCollection(raw)
	kept := keptIDs(payloads)
	for _, b := range p.Blocks {
		if b.NewRecord {
			continue
		}
		if !kept[b.ID] {
			b.Marked = true
			continue
		}
		var payload *rb.Map
		for _, item := range payloads {
			if rubyToI(item.Get("id")) == b.ID {
				payload = item
				break
			}
		}
		if payload == nil || !payload.Has("steps_attributes") {
			continue
		}
		stepKept := keptIDs(payloadCollection(payload.Get("steps_attributes")))
		for _, s := range b.Steps {
			if !s.NewRecord && !stepKept[s.ID] {
				s.Marked = true
			}
		}
	}
}

func fail(class, message string) { panic(&web.StandardError{Class: class, Message: message}) }
