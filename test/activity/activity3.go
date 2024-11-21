package activity

import (
	"context"
	"github.com/tianlin0/plat-lib/logs"
	oneAct "github.com/tianlin0/temporal/activity"
	"go.temporal.io/sdk/activity"
	"time"
)

// Activity3 xx
type Activity3 struct {
}

// Execute ff
func (a *Activity3) Execute(ctx context.Context, input interface{}) (bool, error) {
	name := activity.GetInfo(ctx).ActivityType.Name
	logs.DefaultLogger().Error("Activity3.Execute run!!!!", name, input)
	time.Sleep(2 * time.Second)
	return true, nil
}

// Template ff
func (a *Activity3) Template() string {
	return "Activity3"
}

// GetMethod ff
func (a *Activity3) GetMethod() oneAct.TemplateMethod {
	return func(ctx context.Context, param map[string]interface{}) (map[string]interface{}, error) {
		ret, err := a.Execute(ctx, 222)
		return map[string]interface{}{
			"ret": ret,
		}, err
	}
}
