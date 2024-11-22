package activity

import (
	"context"
	"github.com/tianlin0/plat-lib/curl"
	"github.com/tianlin0/plat-lib/logs"
	oneAct "github.com/tianlin0/temporal/activity"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"net/http"
	"time"
)

// Activity2 vv
type Activity2 struct {
}

// Execute xx
func (a *Activity2) Execute(ctx context.Context, input int) (map[string]interface{}, error) {
	name := activity.GetInfo(ctx).ActivityType.Name

	resp := curl.NewRequest(&curl.Request{
		Url:    "",
		Method: http.MethodGet,
	}).Submit(ctx)

	_ = temporal.NewApplicationError("err23333 error", "", resp)

	logs.DefaultLogger().Error("Activity2.Execute sleep 20s!!!!", input, name)
	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"paasId": "paasId",
		"input":  input,
	}, nil
}

// Template xx
func (a *Activity2) Template() string {
	return "Activity2"
}

// GetMethod ff
func (a *Activity2) GetMethod() oneAct.TemplateMethod {
	return func(ctx context.Context, param map[string]interface{}) (map[string]interface{}, error) {
		return a.Execute(ctx, 222)
	}
}
