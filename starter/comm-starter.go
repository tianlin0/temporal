package starter

import (
	"context"
	"fmt"
	dataConn "github.com/tianlin0/plat-lib/conn"
	"github.com/tianlin0/temporal/activity"
	"github.com/tianlin0/temporal/conn"
	"github.com/tianlin0/temporal/worker"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	v11 "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/workflow"
)

// Config 配置文件
type Config struct {
	Connect        *dataConn.Connect //必填
	CertPath       string
	KeyPath        string
	TaskQueueName  string                      //队列名
	WorkerFlow     interface{}                 //流程
	ActivityList   []activity.TemplateActivity //必填
	ActivityOption *workflow.ActivityOptions
}

type startUp struct {
	registered bool
	cfg        *Config
}

// New 新增
func New(cfg *Config) *startUp {
	su := new(startUp)
	su.cfg = cfg
	return su
}

// Start 启动注册服务
func (su *startUp) Start(useRun bool) error {
	cfg := su.cfg
	if su.registered {
		return fmt.Errorf("cfg %s has registerd", cfg.TaskQueueName)
	}
	if cfg.TaskQueueName == "" ||
		cfg.WorkerFlow == nil ||
		cfg.ActivityList == nil || len(cfg.ActivityList) == 0 {
		return fmt.Errorf("cfg %s param error", cfg.TaskQueueName)
	}

	temporalClient, err := conn.GetTemporalClient(cfg.Connect, cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return err
	}
	cw, err := worker.NewWork(temporalClient, cfg.TaskQueueName)
	if err != nil {
		return err
	}
	err = cw.RegisterWorkflow(cfg.WorkerFlow).RegisterActivityList(cfg.ActivityList).Start(useRun)
	if err == nil {
		su.registered = true
	}
	return err
}

// Submit 提交一条流程
// args 为执行 cfg.WorkerFlow 除了ctx后面的参数列表
func (su *startUp) Submit(ctx context.Context, workflowId string, args ...interface{}) (client.WorkflowRun, error) {
	cfg := su.cfg
	if cfg.TaskQueueName == "" ||
		cfg.WorkerFlow == nil {
		return nil, fmt.Errorf("cfg %s param error", cfg.TaskQueueName)
	}

	temporalClient, err := conn.GetTemporalClient(cfg.Connect, cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, err
	}
	cw, err := worker.NewWork(temporalClient, cfg.TaskQueueName)
	if err != nil {
		return nil, err
	}
	wr, err := cw.ExecuteWorkflow(ctx, workflowId, cfg.WorkerFlow, args...)
	if err != nil {
		return wr, activity.New().GetErrorByTemporalError(err)
	}
	return wr, nil
}

// GetAllLogList 获取指定任务队列中工作流的全部日志或者某个运行中的步骤的日志列表。
// 如果 isAll 为 false，则只返回当前运行中的步骤的日志；否则返回所有历史步骤的日志。
// 参数：
//   - taskQueue: 任务队列名称
//   - workflowId: 工作流 ID
//   - runId: 当前运行的 ID (empty for all runs)
//   - isAll: 是否获取所有历史步骤的日志 (true 时返回完整的日志列表，false 时仅返回当前运行中的步骤的日志)
//
// 返回：
//   - 如果 isAll 为 true，则返回包含 "steps" 和 "logs" 字段的映射，其中 "steps" 是任务队列中所有步骤的信息，"logs" 是所有历史步骤的日志;
//     如果 isAll 为 false，则返回 *([]*OneLog)，即仅包含当前运行中的步骤的日志的切片。
type actHistoryEvent struct {
	StartEvent       *actOneHistoryEvent
	CompleteEvent    *actOneHistoryEvent
	ActivityTypeName string
	Payloads         []*common.Payload
	FailureMessage   string
	TaskFailed       bool
	UseTime          int64 //使用了多长时间，毫秒
}

type actOneHistoryEvent struct {
	HistoryEvent *historypb.HistoryEvent
	TaskQueue    string
	WorkflowId   string
	RunId        string
}

func getAllHistoryEvent(temporalClient client.Client, taskQueue string, workflowId, runId string) []*actOneHistoryEvent {
	iter := temporalClient.GetWorkflowHistory(context.Background(), workflowId, runId, false, enums.HISTORY_EVENT_FILTER_TYPE_UNSPECIFIED)
	historyList := make([]*historypb.HistoryEvent, 0)
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			break
		}
		historyList = append(historyList, event)
	}

	actList := make([]*actOneHistoryEvent, 0)

	if len(historyList) == 0 {
		return actList
	}

	for _, one := range historyList {
		actList = append(actList, &actOneHistoryEvent{
			HistoryEvent: one,
			TaskQueue:    taskQueue,
			WorkflowId:   workflowId,
			RunId:        runId,
		})
	}

	//子流程
	childWorkflowList := make([]*historypb.HistoryEvent, 0)
	for _, one := range historyList {
		child := one.GetChildWorkflowExecutionStartedEventAttributes()
		if child != nil {
			childWorkflowList = append(childWorkflowList, one)
			continue
		}
	}

	//子流程里取得列表
	if len(childWorkflowList) > 0 {
		for _, child := range childWorkflowList {
			childAttr := child.GetChildWorkflowExecutionStartedEventAttributes()
			childObject := childAttr.GetWorkflowExecution()
			childList := getAllHistoryEvent(temporalClient, taskQueue, childObject.GetWorkflowId(), childObject.GetRunId())
			actList = append(actList, childList...)
		}
	}
	return actList
}

// GetAllLogList 获取历史记录
func (su *startUp) GetAllLogList(taskQueue string, workflowId, runId string) ([]*actHistoryEvent, error) {
	cfg := su.cfg
	temporalClient, err := conn.GetTemporalClient(cfg.Connect, cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, err
	}

	if taskQueue == "" {
		return nil, fmt.Errorf("taskQueue is empty")
	}

	historyList := getAllHistoryEvent(temporalClient, taskQueue, workflowId, runId)

	allActHistoryList := make([]*actHistoryEvent, 0)

	if len(historyList) == 0 {
		return allActHistoryList, nil
	}

	for _, one := range historyList {
		attr := one.HistoryEvent.GetActivityTaskScheduledEventAttributes()
		if attr == nil {
			continue
		}
		activityName := attr.ActivityType.Name
		if activityName != "" {
			allActHistoryList = append(allActHistoryList, &actHistoryEvent{
				StartEvent:       one,
				ActivityTypeName: activityName,
			})
		}
	}

	//根据EventId 查询结果EventId
	for _, oneLog := range allActHistoryList {
		//首先查找是否完成的情况
		isCompleted := false
		for _, one := range historyList {
			comp := one.HistoryEvent.GetActivityTaskCompletedEventAttributes()
			if comp == nil {
				continue
			}
			if comp.ScheduledEventId == oneLog.StartEvent.HistoryEvent.EventId &&
				one.TaskQueue == oneLog.StartEvent.TaskQueue &&
				one.WorkflowId == oneLog.StartEvent.WorkflowId &&
				one.RunId == oneLog.StartEvent.RunId {

				result := comp.GetResult()
				dataList := result.GetPayloads()
				oneLog.CompleteEvent = one
				oneLog.Payloads = dataList
				isCompleted = true
				break
			}
		}

		if isCompleted {
			continue
		}
		//如果没有完成，则查询是否执行错误了
		for _, one := range historyList {
			comp := one.HistoryEvent.GetActivityTaskFailedEventAttributes()
			if comp == nil {
				continue
			}
			if comp.ScheduledEventId == oneLog.StartEvent.HistoryEvent.EventId &&
				one.TaskQueue == oneLog.StartEvent.TaskQueue &&
				one.WorkflowId == oneLog.StartEvent.WorkflowId &&
				one.RunId == oneLog.StartEvent.RunId {

				isCompleted = true
				result := comp.Failure
				oneLog.CompleteEvent = one
				oneLog.FailureMessage = result.GetMessage()
				oneLog.TaskFailed = true
				break
			}

		}
		if isCompleted {
			continue
		}
	}
	//需要计算耗时
	for _, one := range allActHistoryList {
		if one.CompleteEvent != nil {
			one.UseTime = one.CompleteEvent.HistoryEvent.GetEventTime().AsTime().
				Sub(one.StartEvent.HistoryEvent.GetEventTime().AsTime()).Milliseconds()
		}
	}
	return allActHistoryList, nil
}

type workflowState struct {
	Status     v11.WorkflowExecutionStatus
	WorkflowId string
	RunId      string
}

// GetAllWorkflowStatus 获取状态
func (su *startUp) GetAllWorkflowStatus(workflowId, runId string) ([]*workflowState, error) {
	cfg := su.cfg
	temporalClient, err := conn.GetTemporalClient(cfg.Connect, cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, err
	}

	descResp, err := temporalClient.DescribeWorkflowExecution(context.Background(), workflowId, runId)
	if err != nil {
		return nil, err
	}
	allState := make([]*workflowState, 0)
	allState = append(allState, &workflowState{
		Status:     descResp.GetWorkflowExecutionInfo().GetStatus(),
		WorkflowId: workflowId,
		RunId:      runId,
	})

	childWorkflow := descResp.GetPendingChildren()
	if childWorkflow != nil {
		for _, one := range childWorkflow {
			oneDescResp, errTemp := temporalClient.DescribeWorkflowExecution(context.Background(), one.GetWorkflowId(), one.GetRunId())
			if errTemp != nil {
				continue
			}
			allState = append(allState, &workflowState{
				Status:     oneDescResp.GetWorkflowExecutionInfo().GetStatus(),
				WorkflowId: one.GetWorkflowId(),
				RunId:      one.GetRunId(),
			})
		}
	}
	return allState, nil
}
