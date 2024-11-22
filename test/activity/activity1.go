package activity

import (
	"context"
	"fmt"
	"github.com/tianlin0/plat-lib/curl"
	"github.com/tianlin0/plat-lib/logs"
	oneAct "github.com/tianlin0/temporal/activity"
	"go.temporal.io/sdk/activity"
	"net/http"
)

// Activity1 fdd
type Activity1 struct {
}

// GetActivity1Name xx
func (a *Activity1) GetActivity1Name(ctx context.Context, input string, key int, mm []int, id ...string) (map[string]interface{}, error) {
	name := activity.GetInfo(ctx).ActivityType.Name
	logs.DefaultLogger().Debug(activity.GetInfo(ctx))

	for _, one := range id {
		fmt.Println(one)
	}

	curl.NewRequest(&curl.Request{
		Url:    "",
		Method: http.MethodGet,
	}).Submit(ctx)

	logs.DefaultLogger().Error("Activity1.Execute run!!!!", name, input, key)

	aa := ctx.Value("aaa")

	return map[string]interface{}{
		"name":  "new Activity1",
		"input": input,
		"key":   key,
		"aaa":   aa,
	}, nil
}

// Template ff
func (a *Activity1) Template() string {
	return "Activity1"
}

// GetMethod ff
func (a *Activity1) GetMethod() oneAct.TemplateMethod {
	return func(ctx context.Context, param map[string]interface{}) (map[string]interface{}, error) {
		return a.GetActivity1Name(ctx, "aaa", 222, []int{1, 2, 3})
	}
}
