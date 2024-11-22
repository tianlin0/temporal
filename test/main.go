package main

import (
	"fmt"
	"github.com/tianlin0/plat-lib/conn"
	"github.com/tianlin0/temporal/activity"
	"github.com/tianlin0/temporal/starter"
	act "github.com/tianlin0/temporal/test/activity"
	"github.com/tianlin0/temporal/workflow"
)

const (
	temporalHost = "127.0.0.1"
	temporalPort = "7233"
)

func main1() {
	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		TaskQueueName: "test-template",
		WorkerFlow:    workflow.New().TemplateWorkflow,
		ActivityList: []activity.TemplateActivity{
			new(act.Activity1),
			new(act.Activity2),
			new(act.Activity3),
		},
	}

	err := starter.New(cfg).Start(false)

	fmt.Println(err)
}
func main() {
	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		TaskQueueName: "test-dsl",
		WorkerFlow:    workflow.New().GetDslWorkflow().DslWorkflow,
		ActivityList: []activity.TemplateActivity{
			new(act.Activity1),
			new(act.Activity2),
			new(act.Activity3),
		},
	}

	err := starter.New(cfg).Start(true)

	fmt.Println(err)
}
