package filter

import (
	"strconv"
	"strings"
)

type TagValue string
func (v TagValue) AsInt() int64 {
	val, _ := strconv.ParseInt(string(v), 0, 64)
	return val
}
func (v TagValue) AsFloat() float64 {
	val, _ := strconv.ParseFloat(string(v), 64)
	return val
}
func (v TagValue) AsIntSlice() []int64 {
	parts := strings.Split(string(v),",")
	res := make([]int64, len(parts))
	for i, part := range parts {
		val, _ := strconv.ParseInt(part, 0, 64)
		res[i] = val
	}
	return res
}
func (v TagValue) AsFloatSlice() []float64 {
	parts := strings.Split(string(v),",")
	res := make([]float64, len(parts))
	for i, part := range parts {
		val, _ := strconv.ParseFloat(part, 64)
		res[i] = val
	}
	return res
}
func (v TagValue) AsStringSlice() []string {
	return strings.Split(string(v), ",")
}

// Tags2map converts a map[string]TagValue back to a map[string]string (for debug purposes only)
func Tags2map(tags map[string]TagValue) map[string]string {
	t := make(map[string]string)
	for k, v := range tags {
		t[k] = string(v)
	}
	return t
}

// Map2tags converts a map[string]string to a map[string]TagValue to be used in the filter Env (providing helper
// functions on TagValue to convert to number or array of numbers or strings).
func Map2tags(tags map[string]string) map[string]TagValue {
	t := make(map[string]TagValue)
	for k, v := range tags {
		t[k] = TagValue(v)
	}
	return t
}

