package activity

import (
	"context"
	"fmt"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/utils"
	"net/http"
	"time"

	"go.temporal.io/sdk/activity"
)

// Activity4 xx
type Activity4 struct {
}

func (a *Activity4) Execute(ctx context.Context, header http.Header, input interface{}) (map[string]interface{}, error) {
	name := activity.GetInfo(ctx).ActivityType.Name
	logs.DefaultLogger().Info("Activity4.Execute run!!!!", name, input)

	time.Sleep(5 * time.Second)

	err := fmt.Errorf("aaaa")

	return map[string]interface{}{
		"ayncId": "resultId:" + utils.NewUUID(),
	}, err
}

func (a *Activity4) Name() string {
	return "Activity4"
}
