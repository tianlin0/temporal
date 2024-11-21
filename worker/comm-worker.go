package worker

import (
	"context"
	"fmt"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/utils"
	"github.com/tianlin0/temporal/activity"
	act "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"reflect"
	"time"
)

type commWork struct {
	client         client.Client
	queueName      string
	workers        worker.Worker
	isRegisterAct  bool
	isRegisterFlow bool
}

// NewWork 新的worker
func NewWork(c client.Client, taskQueueName string) (*commWork, error) {
	if c == nil || taskQueueName == "" {
		return nil, fmt.Errorf("param error")
	}
	w := worker.New(c, taskQueueName, worker.Options{
		DisableRegistrationAliasing:  false,
		StickyScheduleToStartTimeout: 5 * time.Second, //工作流任务从调度到开始的超时时间，如果超时，则会切换到另外的worker
	})
	cw := &commWork{
		client:    c,
		workers:   w,
		queueName: taskQueueName,
	}
	return cw, nil
}

// RegisterActivityList 注册所有activity
func (cw *commWork) RegisterActivityList(activityList []activity.TemplateActivity) *commWork {
	isRegisterAct := false
	var registerErr error
	cs := activity.New()

	hasRegisterActName := make([]string, 0)

	for i, oneAct := range activityList {
		name := oneAct.Template()
		actFunc := oneAct.GetMethod()
		if name == "" {
			logs.DefaultLogger().Error("act Name return null, number:", i)
			continue
		}
		//actStruct里不能有属性，因为是全局的，有属性会很容易调用错误，这里进行检测
		actValue := reflect.Indirect(reflect.ValueOf(oneAct))
		if actValue.Type().Kind() == reflect.Struct {
			if actValue.Type().NumField() > 0 {
				logs.DefaultLogger().Error("activity 里不能包含属性，因为是全局的，会容易用错, ActName:", name)
				continue
			}
		}

		cw.workers.RegisterActivityWithOptions(actFunc, act.RegisterOptions{
			Name:                       cs.GetActivityName(cw.queueName, name),
			SkipInvalidStructFunctions: true,
		})
		isRegisterAct = true

		hasRegisterActName = append(hasRegisterActName, name)

		err := cs.SetActivityMethodName(cw.queueName, name, actFunc)
		if err != nil {
			registerErr = err
			logs.DefaultLogger().Error("SetActivityMethodName error:", cw.queueName, name, err)
		}
	}

	logs.DefaultLogger().Info("RegisterActivityList:", cw.queueName, conv.String(hasRegisterActName))

	if registerErr == nil && isRegisterAct {
		cw.isRegisterAct = true
	}
	return cw
}

// RegisterWorkflow 注册workflow
func (cw *commWork) RegisterWorkflow(workflowTemplate interface{}) *commWork {
	cw.workers.RegisterWorkflow(workflowTemplate)
	cw.isRegisterFlow = true
	return cw
}

// Start 启动
func (cw *commWork) Start(useRun bool) error {
	if cw.isRegisterAct && cw.isRegisterFlow {
		if useRun {
			err := cw.workers.Run(worker.InterruptCh())
			if err != nil {
				logs.DefaultLogger().Error("Unable to start worker", err)
			}
			return err
		}

		err := cw.workers.Start()
		if err != nil {
			logs.DefaultLogger().Error("Unable to start worker", err)
		}
		return err
	}

	return fmt.Errorf("has no Register")
}

// ExecuteWorkflow 执行一条workflow
func (cw *commWork) ExecuteWorkflow(ctx context.Context, workflowId string, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	if workflowId == "" {
		workflowId = utils.NewUUID()
	} else {
		workflowId = fmt.Sprintf("%s-%s", workflowId, utils.GetRandomString(5))
	}
	workflowId = fmt.Sprintf("%s/%s", cw.queueName, workflowId)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowId,
		TaskQueue: cw.queueName,
	}

	if ctx == nil {
		ctx = context.Background()
	}

	logger := logs.CtxLogger(ctx)

	logger.Info("ExecuteWorkflow:", cw.queueName, workflowId)

	we, err := cw.client.ExecuteWorkflow(ctx, workflowOptions, workflow, args...)
	if err != nil {
		logger.Error("Unable to execute workflow", err)
		//这里表示连接不上temporal服务器，需要将错误内容进行具体化。
		return nil, fmt.Errorf("unable to execute workflow, please check temporal server: %s", err.Error())
	}

	logger.Debug("Started workflow WorkflowID:", we.GetID(), "RunID:", we.GetRunID())
	return we, nil
}

// GetWorker 获取worker
func (cw *commWork) GetWorker() (worker.Worker, error) {
	if cw.isRegisterAct && cw.isRegisterFlow {
		return cw.workers, nil
	}
	return nil, fmt.Errorf("has no Register")
}

//// GetWorkflow 方便activity里进行调用
//func (cw *commWork) GetWorkflow(ctx context.Context) client.WorkflowRun {
//	startOpt := GetStartWorkflowOptions(ctx)
//	if startOpt != nil {
//		return cw.client.GetWorkflow(ctx, startOpt.ID, "")
//	}
//	return nil
//}

//func getActivityMethodList(w worker.Worker) []string {
//	v := reflect.ValueOf(w)
//	var mapValue reflect.Value
//	if v.Kind() == reflect.Ptr && !v.IsNil() {
//		mapValue = v.Elem()
//	} else {
//		mapValue = reflect.ValueOf(w)
//	}
//	temp := mapValue.FieldByName("registry")
//
//	mapKeyList := temp.Elem().FieldByName("activityFuncMap").MapKeys()
//
//	newKeyList := make([]string, 0)
//	for _, one := range mapKeyList {
//		newKeyList = append(newKeyList, one.String())
//	}
//	return newKeyList
//}
