package llm

import "reflect"

// SliceValues reads any slice-shaped value as a []interface{}.
//
// Documents decoded by the Mongo driver carry arrays as primitive.A, a named type, so asserting []interface{}
// yields nil and the caller silently sees "no items" instead of a type mismatch. Every reader of a campaign array
// goes through here so that mistake cannot come back at the next call site.
//
// Returns nil when the value is not a slice or array; a slice that happens to be empty comes back empty, which lets
// callers distinguish "no such field" from "field present but empty".
func SliceValues(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.([]interface{}); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil
	}
	out := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out
}
