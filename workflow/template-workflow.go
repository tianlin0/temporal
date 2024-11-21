package workflow

import (
	cmap "github.com/orcaman/concurrent-map"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

// TemplateWorkflow 运行公共的流程
func (c *commWorkflow) TemplateWorkflow(ctx workflow.Context, actOption *workflow.ActivityOptions, stepsList TemplateStepList, args map[string]interface{}) (cmap.ConcurrentMap, error) {
	if actOption == nil {
		actOption = &workflow.ActivityOptions{
			ScheduleToCloseTimeout: time.Duration(5) * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 1,
				MaximumInterval: time.Duration(2) * time.Second,
			},
		}
	}

	ctx = workflow.WithActivityOptions(ctx, *actOption)
	return (&stepsList).Run(ctx, args)
}
