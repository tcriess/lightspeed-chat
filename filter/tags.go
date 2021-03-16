package filter

import (
	"strconv"
	"strings"

	"github.com/antonmedv/expr"
	"github.com/mitchellh/mapstructure"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/types"
)

// AsInt parses the TagValue as an int, 0 on error
func AsInt(v string) int64 {
	val, _ := strconv.ParseInt(v, 0, 64)
	return val
}

// AsFloat parses the TagValue an a float64, 0.0 on error
func AsFloat(v string) float64 {
	val, _ := strconv.ParseFloat(v, 64)
	return val
}

// AsIntSlice parses the TagValue as a comma-separated slice of int64s (0 in every unparsable item)
func AsIntSlice(v string) []int64 {
	parts := strings.Split(v, ",")
	res := make([]int64, len(parts))
	for i, part := range parts {
		val, _ := strconv.ParseInt(part, 0, 64)
		res[i] = val
	}
	return res
}

// AsFloatSlice parses the TagValue as a comma-separated slice of float64s (0.0 in every unparsable item)
func AsFloatSlice(v string) []float64 {
	parts := strings.Split(v, ",")
	res := make([]float64, len(parts))
	for i, part := range parts {
		val, _ := strconv.ParseFloat(part, 64)
		res[i] = val
	}
	return res
}

// AsStringSlice parses the TagValue as a comma-separated slice of strings
func AsStringSlice(v string) []string {
	return strings.Split(v, ",")
}

// UpdateTags modifies the tags map (required to be non-nil!) according to the given set of updates.
// Each types.TagUpdate contains the name of the map entry to update, the type of the entry (one of the
// TagValueType* consts), an index (for the slice types) and an expression to be applied (the tags are accessible
// as "Tags", the helper functions for type conversion of the entries are listed above).
// UpdateTags is supposed to be called from within the persister transactions in persister.UpdateUserTags and
// persister.UpdateRoomTags
func UpdateTags(tags map[string]string, updates []*types.TagUpdate) []bool {
	resOk := make([]bool, len(updates))
	if tags == nil {
		return resOk
	}
	env := TagsEnv{
		Tags: tags,
		AsInt:         AsInt,
		AsFloat:       AsFloat,
		AsStringSlice: AsStringSlice,
		AsIntSlice:    AsIntSlice,
		AsFloatSlice:  AsFloatSlice,
	}
	for i, update := range updates {
		res, err := expr.Eval(update.Expression, env)
		if err != nil {
			globals.AppLogger.Error("could not evaluate expression", "expression", update.Expression, "env", env)
			continue
		}
		helperMap := map[string]interface{}{"value": res}
		switch update.Type {
		case types.TagValueTypeString:
			resHelperMap := struct {
				Value string `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			tags[update.Name] = resHelperMap.Value

		case types.TagValueTypeInt:
			resHelperMap := struct {
				Value int64 `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			tags[update.Name] = strconv.FormatInt(resHelperMap.Value, 10)

		case types.TagValueTypeFloat:
			resHelperMap := struct {
				Value float64 `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			tags[update.Name] = strconv.FormatFloat(resHelperMap.Value, 'f', -1, 64)

		case types.TagValueTypeStringSlice:
			resHelperMap := struct {
				Value string `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			prevValueStr, _ := tags[update.Name]
			prevValues := AsStringSlice(prevValueStr)
			if len(prevValues) > update.Index {
				prevValues[update.Index] = resHelperMap.Value
				tags[update.Name] = strings.Join(prevValues, ",")
			}

		case types.TagValueTypeIntSlice:
			resHelperMap := struct {
				Value int64 `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			prevValueStr, _ := tags[update.Name]
			prevValues := AsIntSlice(prevValueStr)
			if len(prevValues) > update.Index {
				prevValues[update.Index] = resHelperMap.Value
				prevValuesStr := make([]string, len(prevValues))
				for i, v := range prevValues {
					prevValuesStr[i] = strconv.FormatInt(v, 10)
				}
				tags[update.Name] = strings.Join(prevValuesStr, ",")
			}

		case types.TagValueTypeFloatSlice:
			resHelperMap := struct {
				Value float64 `mapstructure:"value"`
			}{}
			err := mapstructure.WeakDecode(helperMap, &resHelperMap)
			if err != nil {
				globals.AppLogger.Error("could not decode result", "error", err)
				continue
			}
			prevValueStr, _ := tags[update.Name]
			prevValues := AsFloatSlice(prevValueStr)
			if len(prevValues) > update.Index {
				prevValues[update.Index] = resHelperMap.Value
				prevValuesStr := make([]string, len(prevValues))
				for i, v := range prevValues {
					prevValuesStr[i] = strconv.FormatFloat(v, 'f', -1, 64)
				}
				tags[update.Name] = strings.Join(prevValuesStr, ",")
			}
		}
		resOk[i] = true
	}
	return resOk
}
