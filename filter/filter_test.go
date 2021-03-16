package filter

import (
	"testing"

	"github.com/antonmedv/expr"
	"github.com/stretchr/testify/assert"
	"github.com/tcriess/lightspeed-chat/types"
)

func TestUpdateTags(t *testing.T) {
	tags := map[string]string{"TEST": "0.1", "TestSlice": "1,2,3"}
	updates := []*types.TagUpdate{&types.TagUpdate{
		Name:       "TEST",
		Type:       types.TagValueTypeFloat,
		Expression: `AsFloat(Tags["TEST"])+17`,
	}, &types.TagUpdate{
		Name:       "TestSlice",
		Type:       types.TagValueTypeIntSlice,
		Index:      1,
		Expression: `AsIntSlice(Tags["TestSlice"])[1]+17`,
	}}
	UpdateTags(tags, updates)
	assert.Equal(t, "17.1", tags["TEST"])
	assert.Equal(t, "1,19,3", tags["TestSlice"])

	tags = map[string]string{"TEST": "17"}
	updates = []*types.TagUpdate{&types.TagUpdate{
		Name:       "TEST",
		Type:       types.TagValueTypeInt,
		Expression: `AsInt(Tags["TEST"])>=17 ? AsInt(Tags["TEST"])-17 : 0/0`,
	}}
	oks := UpdateTags(tags, updates)
	assert.Equal(t, "0", tags["TEST"])
	assert.Equal(t, true, oks[0])
	updates = []*types.TagUpdate{&types.TagUpdate{
		Name:       "TEST",
		Type:       types.TagValueTypeInt,
		Expression: `AsInt(Tags["TEST"])>=17 ? AsInt(Tags["TEST"])-17 : 0/0`,
	}}
	oks = UpdateTags(tags, updates)
	assert.Equal(t, "0", tags["TEST"])
	assert.Equal(t, false, oks[0])
}

func TestTargetFilter(t *testing.T) {
	targetFilter := `AsInt(Source.User.Tags["Test"])==42`
	Event := types.NewEvent(nil, nil, targetFilter, "", "EVENT", nil)
	tags := make(map[string]string)
	tags["Test"] = "42"
	env := Env{
		Room: Room{},
		Source: Source{
			User: User{
				Id:         "user",
				Nick:       "user",
				Language:   "",
				Tags:       tags,
				LastOnline: 0,
			},
			PluginName: "",
		},
		Target:        Target{},
		Created:       0,
		Language:      "",
		Name:          Event.Name,
		Tags:          nil,
		AsInt:         AsInt,
		AsFloat:       AsFloat,
		AsStringSlice: AsStringSlice,
		AsIntSlice:    AsIntSlice,
		AsFloatSlice:  AsFloatSlice,
	}
	res, err := expr.Eval(targetFilter, env)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	assert.Equal(t, true, res.(bool))
	targetFilter = `Source.User.Tags["Test"]=="42"`
	res, err = expr.Eval(targetFilter, env)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	assert.Equal(t, true, res.(bool))
	targetFilter = `Source.User.Tags["Test"]=="41"`
	res, err = expr.Eval(targetFilter, env)
	if err != nil {
		t.Fatalf("error: %s", err)
	}
	assert.Equal(t, false, res.(bool))
}
