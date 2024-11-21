package test1

import (
	"context"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/utils"
	"net/http"
	"time"

	"go.temporal.io/sdk/activity"
)

type Activity4 struct {
}

func (a *Activity4) Execute(ctx context.Context, header http.Header, input interface{}) (map[string]interface{}, error) {
	name := activity.GetInfo(ctx).ActivityType.Name
	logs.DefaultLogger().Info("Activity5.Execute run!!!!", name, input)

	time.Sleep(5 * time.Second)

	return map[string]interface{}{
		"Activity5": "resultId:" + utils.NewUUID(),
	}, nil
}

func (a *Activity4) Name() string {
	return "Activity5"
}
