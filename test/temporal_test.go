package main

import (
	"context"
	"encoding/base64"
	"fmt"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/tianlin0/plat-lib/conn"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/temporal/activity"
	"github.com/tianlin0/temporal/starter"
	act "github.com/tianlin0/temporal/test/activity"
	"github.com/tianlin0/temporal/workflow"
	"log"
	"os"
	"testing"
)

func TestDslWorkflowStart(t *testing.T) {
	data, err := os.ReadFile("workflow1.yaml")
	if err != nil {
		log.Fatalln("failed to load dsl config file", err)
		return
	}
	var dslWorkflow workflow.DslWorkflow
	if err := yaml.Unmarshal(data, &dslWorkflow); err != nil {
		fmt.Println("failed to unmarshal dsl config", err)
		return
	}

	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		TaskQueueName: "test-dsl",
		WorkerFlow:    workflow.New().GetDslWorkflow().DslWorkflow,
	}

	ctx := context.Background()

	cu, err := starter.New(cfg).Submit(ctx, "delete-cd-test", nil, &dslWorkflow)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	ret := make(map[string]interface{})

	cu.Get(ctx, &ret)

	fmt.Println(conv.String(ret))

	fmt.Println(cu.GetID(), cu.GetRunID(), err)
}

func TestTemplateWorkflowStart(t *testing.T) {
	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		TaskQueueName: "test-template",
		WorkerFlow:    workflow.New().TemplateWorkflow,
	}

	stepList := []*wfv1.ParallelSteps{
		{
			Steps: []wfv1.WorkflowStep{
				{
					Name:     "act1",
					Template: "Activity1",
					OnExit:   "return",
				},
				{
					Name:     "act2",
					Template: "Activity2",
					OnExit:   "exit",
				},
			},
		},
		{
			Steps: []wfv1.WorkflowStep{
				{
					Name:     "act3",
					Template: "Activity3",
				},
			},
		},
	}

	ctx := context.Background()

	cu, err := starter.New(cfg).Submit(ctx, "test-template-id", nil, stepList, map[string]interface{}{
		"act1": []interface{}{"tiantian", 5},
		"act2": map[string]interface{}{
			"name":     "12345",
			"paasName": "45678",
		},
		"act3": []interface{}{"333"},
	})

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	ret := make(map[string]interface{})

	cu.Get(ctx, &ret)

	fmt.Println(conv.String(ret))
	fmt.Println(cu.GetID(), cu.GetRunID(), err)
}

func TestEncodeDslWorkflow(t *testing.T) {
	folderName := "xml-store"

	dir, err := os.ReadDir(folderName)
	if err != nil {
		return
	}
	for _, one := range dir {
		data, err := os.ReadFile(folderName + "/" + one.Name())
		if err != nil {
			log.Fatalln("failed to load dsl config file", err)
			return
		}
		str := base64.StdEncoding.EncodeToString(data)
		fmt.Println(one.Name() + ":" + str)
	}
}
func TestDecodeDslWorkflow(t *testing.T) {
	dslXml := "dmFyaWFibGVzOgogIGFjdDE6CiAgICBwYWFzSWQ6ICJtbW1tbSIKICBhY3QyOgogICAgcGFhc05hbWU6ICJiYmJiYiIKICBwYWFzTmFtZTogcGFhc05hbWVfMTExMTEKICBwcm9qZWN0TmFtZTogcHJvamVjdE5hbWVfMjIyMjIKCnJvb3Q6CiAgc2VxdWVuY2U6CiAgICAtIGFjdGl2aXR5OgogICAgICAgIGlkOiBhY3QxCiAgICAgICAgdGVtcGxhdGU6IEFjdGl2aXR5MQogICAgICAgIGFyZ3VtZW50czoKICAgICAgICAgIHByb2plY3ROYW1lOiAie3t2YXJpYWJsZXMucHJvamVjdE5hbWV9fSIKICAgICAgY29udHJvbDoKICAgICAgICBvbmV4aXQ6ICJyZXR1cm4iCiAgICAgICAgd2hlbjogIjE9PTEiCiAgICAgIHBhcmFsbGVsOgogICAgICAgIC0gYWN0aXZpdHk6CiAgICAgICAgICAgIGlkOiBhY3QyCiAgICAgICAgICAgIHRlbXBsYXRlOiBBY3Rpdml0eTIKICAgICAgICAgICAgYXJndW1lbnRzOgogICAgICAgICAgICAgIHByb2plY3ROYW1lOiAie3t2YXJpYWJsZXMucHJvamVjdE5hbWV9fSIKICAgICAgICAtIGFjdGl2aXR5OgogICAgICAgICAgICBpZDogYWN0MwogICAgICAgICAgICB0ZW1wbGF0ZTogQWN0aXZpdHkzCiAgICAgICAgICAgIGFyZ3VtZW50czoKICAgICAgICAgICAgICBwYWFzTmFtZTogInt7YWN0MS5hcmd1bWVudHMucHJvamVjdE5hbWV9fSIKCnJlc3BvbnNlczoKICBhcmNJZDY6ICJ7e2FjdDEucmVzcG9uc2VzLmlucHV0fX0iCg=="
	cm := workflow.New()
	data, err := cm.GetDslWorkflow().GetDslWorkflowFromBase64(dslXml, false)
	fmt.Println(data, err)
}
func TestGetLogList(t *testing.T) {
	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		WorkerFlow: workflow.New().GetDslWorkflow().DslWorkflow,
		ActivityList: []activity.TemplateActivity{
			new(act.Activity1),
			new(act.Activity2),
			new(act.Activity3),
		},
	}

	//gdp-dsl/create-paas|gdp-dsl/create-paas/10pt4c6zn474-f97aw|69bbcfdc-d9be-4aa6-a7ef-20bc1b535783
	aa, err := starter.New(cfg).GetAllLogList("test-dsl", "test-dsl/delete-cd-test-wbejj", "bb99b39a-bf45-4639-8ddf-eaa29954c17f")

	fmt.Println(aa, err)
}

func TestGetLogList2(t *testing.T) {
	cfg := &starter.Config{
		Connect: &conn.Connect{
			Host: temporalHost,
			Port: temporalPort,
		},
		WorkerFlow: workflow.New().GetDslWorkflow().DslWorkflow,
		ActivityList: []activity.TemplateActivity{
			new(act.Activity1),
			new(act.Activity2),
			new(act.Activity3),
		},
	}

	//gdp-dsl/swoole-php|gdp-dsl/swoole-php/gi1abcy3q9s-p1i0g|be5f5f2d-038a-47fb-941c-f144528eaf6f
	//gdp-dsl/create-paas|gdp-dsl/create-paas/10pt4c6zn474-f97aw|69bbcfdc-d9be-4aa6-a7ef-20bc1b535783
	aa, err := starter.New(cfg).GetAllLogList("xml-store/delete-cd.yaml", "gdp-dsl/swoole-php/gi1abcy3q9s-p1i0g", "be5f5f2d-038a-47fb-941c-f144528eaf6f")

	fmt.Println(aa, err)
}
func TestGetError(t *testing.T) {
	//err := fmt.Errorf("activity error (type: gdp-dsl/swoole-php/add-paas, scheduledEventID: 5, startedEventID: 6, identity: 1@gdp-appserver-go-dev-v3-846dd7d9bb-dgj74@): 在该项目(dffds): 下英文名称[fdsfdf2223-gdpdev]已经存在了")
	err1 := fmt.Errorf("未定义的错误码 (type: derek-proj-gateway-gdppre.erere-gdppre, retryable: true) (type: derek-proj-gateway-gdppre.erere-gdppre, retryable: true)")

	commInt := activity.New()
	//errMsg := commInt.GetErrorByTemporalError(err)
	errMsg1 := commInt.GetErrorByTemporalError(err1)

	//fmt.Println(errMsg)
	fmt.Println(errMsg1)
}
