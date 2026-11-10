package campaignspec

import (
	"fmt"
	"reflect"
	"strings"
)

// neighborSliceAny returns v as []interface{} for any slice type.
//
// MongoDB decodes arrays into primitive.A, which is a named slice type and therefore does not match a
// `case []interface{}` type switch. Code that reads a stored campaign document must not assume the plain type: doing so
// silently yields "no regions, no places", and the world block disappears from every prompt.
func neighborSliceAny(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if list, ok := v.([]interface{}); ok {
		return list
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out
}

func stringFrom(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
