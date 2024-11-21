package workflow

import (
	"context"
	"encoding/base64"
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/temporal/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"gopkg.in/yaml.v3"
	"net/url"
	"time"
)

type dslWorkflow struct {
}

// DslWorkflow 运行公共的流程
func (c *dslWorkflow) DslWorkflow(ctx workflow.Context, actOption *workflow.ActivityOptions, dslWorkflow *DslWorkflow) (map[string]interface{}, error) {
	if dslWorkflow == nil {
		return nil, fmt.Errorf("dslWorkflow is null")
	}

	logs.DefaultLogger().Info("DslWorkflow_actOption:", actOption)

	if actOption == nil {
		actOption = &workflow.ActivityOptions{
			ScheduleToCloseTimeout: 30 * time.Minute,
			ScheduleToStartTimeout: 10 * time.Minute,
			StartToCloseTimeout:    10 * time.Minute,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:        time.Second,
				MaximumAttempts:        1,
				NonRetryableErrorTypes: []string{},
			},
		}
	}

	ctx = workflow.WithRetryPolicy(ctx, temporal.RetryPolicy{
		InitialInterval:        time.Second,
		MaximumAttempts:        1,
		NonRetryableErrorTypes: []string{},
	})

	ctx = workflow.WithActivityOptions(ctx, *actOption)

	// 所有参数
	bindings := cmap.New()
	if dslWorkflow.Variables != nil {
		bindings.Set(activity.Variables, dslWorkflow.Variables)
	}

	logger := logs.DefaultLogger()
	if newCtx, ok := ctx.(context.Context); ok {
		logger = logs.CtxLogger(newCtx)
	}

	//设置变量到所有流程的参数中
	dslWorkflow, err := dslWorkflow.SetVariablesToAll(bindings)
	if err != nil {
		logger.Error("DslWorkflow SetVariablesToAll error:", err)
	}

	ret, err := dslWorkflow.Root.Execute(ctx, bindings)
	if err != nil {
		return nil, err
	}
	retMap := make(map[string]interface{})
	if dslWorkflow.Responses != nil && len(dslWorkflow.Responses) > 0 { //表示需要设置返回值
		//返回特定的值
		cm := New()
		retMap, err = cm.GetOutputMap(dslWorkflow.Responses, map[string]interface{}{}, ret)
		if err != nil {
			return nil, err
		}
	} else {
		// 如果没有设置，则默认不返回，确保数据不会泄露
		logger.Info("dslWorkflow.Root.Execute result:", ret)
		//comm := New()
		//retMap = comm.ChangeConcurrentMapToMap(ret)
	}
	return retMap, nil
}

func (c *dslWorkflow) GetDslWorkflowFromBase64(dslXml string, escape bool) (*DslWorkflow, error) {
	dslContent, err := base64.StdEncoding.DecodeString(dslXml)
	if err != nil {
		return nil, err
	}

	var decodedString = string(dslContent)
	if escape {
		decodedStringTemp, err := url.QueryUnescape(string(dslContent))
		if err != nil {
			return nil, err
		}
		decodedString = decodedStringTemp
	}

	var dslWorkflowTemp DslWorkflow
	if err = yaml.Unmarshal([]byte(decodedString), &dslWorkflowTemp); err != nil {
		return nil, err
	}
	return &dslWorkflowTemp, nil
}
