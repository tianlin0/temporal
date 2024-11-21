package workflow

import (
	"context"
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tianlin0/plat-lib/cond"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/plat-lib/goroutines"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/temporal/activity"
	"github.com/tidwall/gjson"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"
	"strings"
	"time"
)

type (
	DslWorkflow struct {
		Variables  map[string]interface{} `json:"variables,omitempty"`  //传入的所有变量参数，包括可以设置某一步的参数
		Root       Statement              `json:"root,omitempty"`       //启动的根目录
		Activities []*OneActivity         `json:"activities,omitempty"` //公共的activity资源，用于公共执行的部分,比如公共打日志
		Responses  map[string]interface{} `json:"responses,omitempty"`  //请求返回的内容
	}

	OneActivity struct {
		Activity *ActivityInvocation `json:"activity,omitempty"`
	}

	Control struct {
		When        string `json:"when,omitempty"`        //执行的前提条件
		OnExit      string `json:"onExit,omitempty"`      //是否执行完当前的Activity后，就直接返回，后续的则子流程
		Wait        string `json:"wait,omitempty"`        //是否需要有等待的Channel
		SeqPriority bool   `json:"seqPriority,omitempty"` //Sequence 是否优先
	}

	Statement struct {
		Control  *Control            `json:"control,omitempty"`
		Activity *ActivityInvocation `json:"activity,omitempty"`
		Parallel Parallel            `json:"parallel,omitempty"`
		Sequence Sequence            `json:"sequence,omitempty"`
	}
	ActivityInvocation struct {
		Id        string                 `json:"id,omitempty"`        //定义的流程里不同的名字
		Template  string                 `json:"template,omitempty"`  //调用某一个Activity名字
		Hooks     LifecycleHooks         `json:"hooks,omitempty"`     //activity执行完时的钩子程序
		Arguments map[string]interface{} `json:"arguments,omitempty"` //需要的参数列表,string为传进来的key
		Responses map[string]interface{} `json:"responses,omitempty"` //返回的字段列表,string为返回的key，可以自定义添加内容
	}
	Sequence []*Statement
	Parallel []*Statement

	LifecycleEvent string
	LifecycleHooks map[LifecycleEvent]*ActivityInvocation

	executable interface {
		Execute(ctx workflow.Context, bindings cmap.ConcurrentMap) (cmap.ConcurrentMap, error)
	}
)

const (
	exitLifecycleEvent = "exit"
)

func (lchs LifecycleHooks) getExitHook() *ActivityInvocation {
	hook, ok := lchs[exitLifecycleEvent]
	if ok {
		return hook
	}
	return nil
}

// SetVariablesToAll 设置所有变量到所有参数中
func (t *DslWorkflow) SetVariablesToAll(bindings cmap.ConcurrentMap) (*DslWorkflow, error) {
	//递归设置variables所有参数给所有argument
	t.setAllCommVariablesToArgument(&t.Root, t.Activities, t.Variables)
	//复制公共activity到各个子流程下
	t.setAllActivitiesToRoot(t.Activities, &t.Root)

	comm := New()
	err := comm.ReplaceAllByBindings(t, bindings)
	if err != nil {
		logs.DefaultLogger().Error("makeInputMap:", err)
		return t, err
	}
	newDsl := new(DslWorkflow)
	return newDsl, conv.Unmarshal(t, newDsl)
}

func (t *DslWorkflow) getAllWorkflowIdList(b *Statement, a []*OneActivity) []string {
	allWorkflowId := make([]string, 0)
	t.getRootWorkflowIdList(b, &allWorkflowId)

	if a != nil && len(a) > 0 {
		for _, one := range a {
			if one.Activity != nil {
				allWorkflowId = append(allWorkflowId, one.Activity.Id)
			}
		}
	}
	return allWorkflowId
}

func (t *DslWorkflow) getOneActivityList(b *Statement) []*ActivityInvocation {
	newList := make([]*ActivityInvocation, 0)
	if b.Activity != nil {
		newList = append(newList, b.Activity)
	}
	if b.Sequence != nil && len(b.Sequence) > 0 {
		for _, one := range b.Sequence {
			sList := t.getOneActivityList(one)
			newList = append(newList, sList...)
		}
	}
	if b.Parallel != nil && len(b.Parallel) > 0 {
		for _, one := range b.Parallel {
			sList := t.getOneActivityList(one)
			newList = append(newList, sList...)
		}
	}
	return newList
}
func (t *DslWorkflow) GetAllActivityList() []*ActivityInvocation {
	allList := t.getOneActivityList(&t.Root)
	//去掉重复
	newAllList := make([]*ActivityInvocation, 0)
	for i, one := range allList {
		found := false
		for _, two := range newAllList {
			if one.Id == two.Id {
				found = true
				break
			}
		}
		if !found {
			newAllList = append(newAllList, allList[i])
		}
	}
	return newAllList
}

func (t *DslWorkflow) getRootWorkflowIdList(b *Statement, idList *[]string) {
	if b.Activity != nil {
		if b.Activity.Id != "" {
			*idList = append(*idList, b.Activity.Id)
		}
	}

	if b.Sequence != nil && len(b.Sequence) > 0 {
		for i, _ := range b.Sequence {
			t.getRootWorkflowIdList(b.Sequence[i], idList)
		}
	}

	if b.Parallel != nil && len(b.Parallel) > 0 {
		for i, _ := range b.Parallel {
			t.getRootWorkflowIdList(b.Parallel[i], idList)
		}
	}
}

func (t *DslWorkflow) setAllCommVariablesToArgument(b *Statement, a []*OneActivity, variable map[string]interface{}) {
	// 将公共参数添加到每个activity的参数中
	allWorkflowId := t.getAllWorkflowIdList(b, a)

	t.setCommVariablesToArgument(b, variable, allWorkflowId)

	if variable == nil || len(variable) == 0 {
		return
	}

	cm := New()

	if a != nil && len(a) > 0 {
		for _, one := range a {
			if one.Activity.Arguments == nil {
				one.Activity.Arguments = map[string]interface{}{}
			}
			arguments := cmap.New()
			for k, v := range one.Activity.Arguments {
				arguments.Set(k, v)
			}

			one.Activity.Arguments = cm.GetInputMap(one.Activity.Id, arguments, variable, allWorkflowId)
		}
	}
}

func (t *DslWorkflow) setCommVariablesToArgument(b *Statement, variable map[string]interface{}, allWorkflowId []string) {
	if variable == nil || len(variable) == 0 {
		return
	}

	if allWorkflowId == nil {
		allWorkflowId = []string{}
	}

	if b.Activity != nil {
		if b.Activity.Arguments == nil {
			b.Activity.Arguments = map[string]interface{}{}
		}
		arguments := cmap.New()
		for k, v := range b.Activity.Arguments {
			arguments.Set(k, v)
		}
		cm := New()
		b.Activity.Arguments = cm.GetInputMap(b.Activity.Id, arguments, variable, allWorkflowId)
	}

	if b.Sequence != nil && len(b.Sequence) > 0 {
		for i, _ := range b.Sequence {
			t.setCommVariablesToArgument(b.Sequence[i], variable, allWorkflowId)
		}
	}

	if b.Parallel != nil && len(b.Parallel) > 0 {
		for i, _ := range b.Parallel {
			t.setCommVariablesToArgument(b.Parallel[i], variable, allWorkflowId)
		}
	}
}

func (t *DslWorkflow) setAllActivitiesToRoot(a []*OneActivity, b *Statement) {
	if a == nil || len(a) == 0 {
		return
	}
	if b.Activity != nil {
		if b.Activity.Id != "" {
			for _, one := range a {
				if one.Activity.Id == b.Activity.Id {
					if b.Activity.Template == "" {
						t.copyOneActivity(one.Activity, b.Activity)
					}
				}
			}
		}
	}

	if b.Sequence != nil && len(b.Sequence) > 0 {
		for i, _ := range b.Sequence {
			t.setAllActivitiesToRoot(a, b.Sequence[i])
		}
	}

	if b.Parallel != nil && len(b.Parallel) > 0 {
		for i, _ := range b.Parallel {
			t.setAllActivitiesToRoot(a, b.Parallel[i])
		}
	}
}

func (t *DslWorkflow) copyOneActivity(source *ActivityInvocation, to *ActivityInvocation) {
	if to.Template == "" {
		to.Template = source.Template
	}
	if source.Arguments != nil && len(source.Arguments) > 0 {
		if to.Arguments == nil {
			to.Arguments = make(map[string]interface{})
		}
		for key, data := range source.Arguments {
			if _, ok := to.Arguments[key]; !ok {
				to.Arguments[key] = data
			}
		}
	}
	if source.Responses != nil && len(source.Responses) > 0 {
		if to.Responses == nil {
			to.Responses = make(map[string]interface{})
		}
		for key, data := range source.Responses {
			if _, ok := to.Responses[key]; !ok {
				to.Responses[key] = data
			}
		}
	}
}

// Execute Statement
func (b *Statement) Execute(ctx workflow.Context, bindings cmap.ConcurrentMap) (cmap.ConcurrentMap, error) {
	var err error

	logger := logs.DefaultLogger()
	if newCtx, ok := ctx.(context.Context); ok {
		logger = logs.CtxLogger(newCtx)
	}

	//执行activity前，首先进行条件判断，有条件未满足，则直接报错
	if b.Control != nil {
		//需要有等待的情况
		if b.Control.Wait != "" {
			signalReceived := false
			workflow.Go(ctx, func(ctx workflow.Context) {
				for {
					selector := workflow.NewSelector(ctx)
					selector.AddReceive(workflow.GetSignalChannel(ctx, b.Control.Wait), func(c workflow.ReceiveChannel, more bool) {
						c.Receive(ctx, nil)
						signalReceived = true
					})
					selector.Select(ctx)
				}
			})

			// 等待信号
			err = workflow.Await(ctx, func() bool {
				return signalReceived
			})
			if err != nil {
				return nil, err
			}
		}

		//需要有条件的情况
		if b.Control.When != "" {
			cm := New()
			canRun, err := cm.ShouldExecute(b.Control.When, bindings)
			if err != nil {
				return bindings, err
			}
			if !canRun {
				return bindings, fmt.Errorf("条件未通过，不允许执行")
			}
		}
	}

	// 遇到错误，是否直接退出
	isExit := false
	//是否需要直接返回
	isReturn := false
	if b.Control != nil {
		//需要有条件的情况
		if b.Control.OnExit != "" {
			onExits := strings.Split(b.Control.OnExit, "|")
			if ok, _ := cond.Contains(onExits, "exit"); ok {
				isExit = true
			}
			if ok, _ := cond.Contains(onExits, "return"); ok {
				isReturn = true
			}
		}
	}

	//执行顺序是：前activity，后Parallel，然后再Sequence
	if b.Activity != nil {
		bindings, err = b.Activity.Execute(ctx, bindings)
		if err != nil {
			logger.Error("Statement.execute 1 error:", conv.String(err.Error()))
			return bindings, err
		}
	}

	if isExit && b.Activity != nil {
		//如果执行没有返回正确的值，则表示没有执行成功
		bindingsJson := conv.String(bindings)
		r := gjson.Get(bindingsJson, fmt.Sprintf("%s.%s", b.Activity.Id, activity.Responses)).Exists()
		if !r {
			return bindings, err
		}
	}

	//直接返回
	if isReturn {
		workInfo := workflow.GetInfo(ctx)

		runChildWorkflow := false
		if b.Parallel != nil && len(b.Parallel) > 0 {
			runChildWorkflow = true
		}
		if b.Sequence != nil && len(b.Sequence) > 0 {
			runChildWorkflow = true
		}

		//是否运行子流程
		if runChildWorkflow {
			comm := New()
			childWorkflow := new(DslWorkflow)

			childWorkflow.Root = Statement{
				Control:  nil,
				Activity: nil,
				Parallel: b.Parallel,
				Sequence: b.Sequence,
			}

			err = comm.ReplaceAllByBindings(childWorkflow, bindings)
			if err != nil {
				logger.Error("ExecuteChildWorkflow:", err)
				return bindings, err
			}

			childWorkflowOptions := workflow.ChildWorkflowOptions{
				ParentClosePolicy: enums.PARENT_CLOSE_POLICY_ABANDON,
				Namespace:         workInfo.Namespace,
				TaskQueue:         workInfo.TaskQueueName,
				//WorkflowExecutionTimeout: time.Minute,
				//WorkflowTaskTimeout:      time.Second * 10,
				//WorkflowID:               "", //子流程的workflowID
			}
			childCtx := workflow.WithChildOptions(ctx, childWorkflowOptions)

			childOption := workflow.GetActivityOptions(ctx)
			childOption.WaitForCancellation = false
			childOption.ScheduleToStartTimeout = 5 * time.Minute

			childWorkflowIns := workflow.ExecuteChildWorkflow(childCtx, workInfo.WorkflowType.Name, childOption, childWorkflow, childWorkflow.Variables)

			logger.Debug("ExecuteChildWorkflow 1:", conv.String(time.Now()))

			goroutines.GoAsyncHandler(func(params ...interface{}) {
				err = childWorkflowIns.Get(childCtx, nil)
				logger.Error("ExecuteChildWorkflow childCtx error:", err)
				logger.Debug("ExecuteChildWorkflow 2:", conv.String(time.Now()))
			}, nil)

		}

		return bindings, nil
	}

	if b.Control != nil && b.Control.SeqPriority {
		if b.Sequence != nil {
			bindings, err = b.Sequence.Execute(ctx, bindings)
			if err != nil {
				logger.Error("Sequence.execute 3 error:", conv.String(err.Error()))
				return bindings, err
			}
		}

		if b.Parallel != nil {
			bindings, err = b.Parallel.Execute(ctx, bindings)
			if err != nil {
				logger.Error("Parallel.execute 2 error:", conv.String(err.Error()))
				return bindings, err
			}
		}
	} else {
		if b.Parallel != nil {
			bindings, err = b.Parallel.Execute(ctx, bindings)
			if err != nil {
				logger.Error("Parallel.execute 2 error:", conv.String(err.Error()))
				return bindings, err
			}
		}

		if b.Sequence != nil {
			bindings, err = b.Sequence.Execute(ctx, bindings)
			if err != nil {
				logger.Error("Sequence.execute 3 error:", conv.String(err.Error()))
				return bindings, err
			}
		}
	}

	return bindings, nil
}

func (a *ActivityInvocation) Execute(ctx workflow.Context, bindings cmap.ConcurrentMap) (cmap.ConcurrentMap, error) {
	workflowInfo := workflow.GetInfo(ctx)

	taskQueueName := workflowInfo.TaskQueueName
	templateName := a.Template

	ac := activity.New()
	activityFunc := ac.GetActivityMethodFunc(taskQueueName, templateName)
	if activityFunc == nil {
		return bindings, fmt.Errorf("%s/%s activity not register", taskQueueName, templateName)
	}

	//获取参数
	inputParam, err := a.getActivityInputMap(a.Arguments, bindings)
	if err != nil {
		return bindings, err
	}

	comm := New()
	//input 不能有没有取到的值，否则直接报错
	err = comm.CheckArguments(a.Id, inputParam)
	if err != nil {
		return bindings, err
	}

	//将参数合并到bindings中
	bindings, err = comm.ExtendToBindings(bindings, inputParam, a.Id, activity.Arguments)
	if err != nil {
		return bindings, err
	}

	oneRet, errRet := ac.GetActivityMethodOutput(taskQueueName, templateName)
	if errRet != nil {
		return bindings, errRet
	}

	logger := logs.DefaultLogger()
	if newCtx, ok := ctx.(context.Context); ok {
		logger = logs.CtxLogger(newCtx)
	}

	logger.Info(fmt.Sprintf("%s %s %s param: %s",
		taskQueueName, templateName, a.Id, conv.String(inputParam)))

	err = workflow.ExecuteActivity(ctx,
		ac.GetActivityName(taskQueueName, templateName), inputParam).Get(ctx, oneRet)

	if err != nil {
		//如果是异步，这里就不用返回错误
		return bindings, activity.New().GetErrorByTemporalError(err)
	}

	outputResult := make(map[string]interface{})
	err = conv.Unmarshal(oneRet, &outputResult)
	if err != nil || len(outputResult) == 0 {
		outputResult[activity.Result] = oneRet
	}

	logger.Debug("ActivityInvocation(ctx, &result):", conv.String(outputResult))

	if len(a.Responses) > 0 {
		outputResult, err = comm.GetOutputMap(a.Responses, outputResult, bindings)
		if err != nil {
			return bindings, err
		}
	}

	bindings, err = comm.ExtendToBindings(bindings, outputResult, a.Id, activity.Responses)
	if err != nil {
		return bindings, err
	}

	//执行钩子程序
	if a.Hooks != nil {
		exitHook := a.Hooks.getExitHook()
		if exitHook != nil {
			_, _ = exitHook.Execute(ctx, bindings)
		}
	}

	return bindings, nil
}

func (p Parallel) Execute(ctx workflow.Context, bindings cmap.ConcurrentMap) (cmap.ConcurrentMap, error) {
	childCtx, cancelHandler := workflow.WithCancel(ctx)
	selector := workflow.NewSelector(ctx)
	var activityErr error

	for _, s := range p {
		f := p.executeAsync(s, childCtx, bindings)
		selector.AddFuture(f, func(f workflow.Future) {
			err := f.Get(ctx, nil)
			if err != nil {
				activityErr = err
			}
		})
	}

	for i := 0; i < len(p); i++ {
		selector.Select(ctx) // this will wait for one branch
		if activityErr != nil {
			cancelHandler()
			return bindings, activityErr
		}
	}

	return bindings, nil
}

func (p Parallel) executeAsync(exe executable, ctx workflow.Context, bindings cmap.ConcurrentMap) workflow.Future {
	future, settable := workflow.NewFuture(ctx)
	workflow.Go(ctx, func(ctx workflow.Context) {
		val, err := exe.Execute(ctx, bindings)
		settable.Set(val, err)
	})
	return future
}

func (a *ActivityInvocation) getActivityInputMap(currArguments map[string]interface{}, arguments cmap.ConcurrentMap) (
	map[string]interface{}, error) {
	args := make(map[string]interface{})

	//自定义进行覆盖
	for key, arg := range currArguments {
		if arg != "" {
			args[key] = arg
		}
	}

	comm := New()
	err := comm.ReplaceAllByBindings(&args, arguments)
	if err != nil {
		logs.DefaultLogger().Error("makeInputMap:", err)
		return args, err
	}
	return args, nil
}

func (s Sequence) Execute(ctx workflow.Context, bindings cmap.ConcurrentMap) (cmap.ConcurrentMap, error) {
	logger := logs.DefaultLogger()
	if newCtx, ok := ctx.(context.Context); ok {
		logger = logs.CtxLogger(newCtx)
	}

	var err error
	for _, a := range s {
		bindings, err = a.Execute(ctx, bindings)
		if err != nil {
			logger.Error("Sequence result error:", err)
			return bindings, err
		}
	}
	return bindings, nil
}
