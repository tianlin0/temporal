package workflow

import (
	"fmt"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tianlin0/plat-lib/cond"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/templates"
	"github.com/tianlin0/plat-lib/utils"
	"github.com/tianlin0/temporal/activity"
	"go.temporal.io/sdk/workflow"
	"strings"
)

type (
	TemplateStepList []*wfv1.ParallelSteps
)

// getOneActivity 获取单个Act执行的方法   返回future，接收参数指针，错误
func (t *TemplateStepList) getOneActivity(ctx workflow.Context,
	oneAct wfv1.WorkflowStep, args []interface{}) (workflow.Future, interface{}, error) {
	workflowInfo := workflow.GetInfo(ctx)

	taskQueueName := workflowInfo.TaskQueueName
	templateName := oneAct.Template

	ac := activity.New()
	activityFunc := ac.GetActivityMethodFunc(taskQueueName, templateName)
	if activityFunc == nil {
		return nil, nil, fmt.Errorf("%s/%s activity not register", taskQueueName, templateName)
	}

	//对参数进行设置
	stepArg, err := ac.GetActivityMethodArguments(taskQueueName, templateName, args)
	if err != nil {
		return nil, nil, err
	}

	// 检查是否存在未被替代的变量
	cm := New()
	if err = cm.CheckArguments(oneAct.Name, stepArg); err != nil {
		return nil, nil, err
	}

	if oneAct.When != "" {
		cm := New()
		canRun, err := cm.ShouldExecute(oneAct.When, args...)
		if err != nil {
			return nil, nil, err
		}
		if !canRun {
			return nil, nil, fmt.Errorf("条件未通过，不允许执行")
		}
	}

	oneRet, errRet := ac.GetActivityMethodOutput(taskQueueName, templateName)
	if errRet != nil {
		return nil, nil, errRet
	}

	workflow.GetLogger(ctx).Info(fmt.Sprintf("%s %s %s param: %s",
		taskQueueName, templateName, oneAct.Name, conv.String(stepArg)))

	f := workflow.ExecuteActivity(ctx,
		ac.GetActivityName(taskQueueName, templateName), stepArg...)

	return f, oneRet, nil
}

// executeOneActivity 真正执行单个Act的整个流程
// allArgs 流程前面执行的所有返回和参数值
// inputArgs 用户传入的参数列表
func (t *TemplateStepList) executeOneActivity(ctx workflow.Context,
	oneAct wfv1.WorkflowStep,
	arguments cmap.ConcurrentMap, variable map[string]interface{}) (map[string]interface{}, error) {

	allActivityNames := t.getAllNames()
	cm := New()
	args := cm.GetInputMap(oneAct.Name, arguments, variable, allActivityNames)

	args, err := t.makeInputMap(args, arguments)
	if err != nil {
		return nil, err
	}

	f, ret, err := t.getOneActivity(ctx, oneAct, []interface{}{args})
	if err != nil {
		return nil, err
	}

	comm := New()
	arguments, err = comm.ExtendToBindings(arguments, args, oneAct.Name, activity.Arguments)
	if err != nil {
		return nil, err
	}

	//取得第一个值
	err = f.Get(ctx, ret)
	if err != nil {
		return nil, err
	}

	retMap := make(map[string]interface{})
	err = conv.Unmarshal(ret, &retMap)
	if err != nil {
		return nil, err
	}

	arguments, err = comm.ExtendToBindings(arguments, args, oneAct.Name, activity.Responses)
	if err != nil {
		return nil, err
	}

	//执行钩子程序
	if oneAct.Hooks != nil {
		exitHook := oneAct.Hooks.GetExitHook()
		if exitHook != nil {
			exitAct := wfv1.WorkflowStep{}
			exitAct.Template = exitHook.Template
			exitAct.Name = ""
			_, _ = t.executeOneActivity(ctx, exitAct, arguments, variable)
		}
	}

	return retMap, nil
}

func (t *TemplateStepList) getAllNames() []string {
	allNames := make([]string, 0)
	for _, one := range *t {
		for _, oneAct := range one.Steps {
			allNames = utils.AppendUnique(allNames, oneAct.Name)
		}
	}
	return allNames
}

func (t *TemplateStepList) makeInputMap(argNames map[string]interface{}, arguments cmap.ConcurrentMap) (
	map[string]interface{}, error) {
	args := make(map[string]interface{})

	//自定义进行覆盖
	for key, arg := range argNames {
		if arg != "" {
			args[key] = arg
		}
	}

	comm := New()
	bindingsMap := comm.ChangeConcurrentMapToMap(arguments)

	argsMapList := conv.ToKeyListFromMap(bindingsMap)
	allParamStr := conv.String(args)
	tmp := templates.NewTemplate(allParamStr)
	allParamStrRet, err := tmp.Replace(argsMapList)
	if err != nil {
		logs.DefaultLogger().Error("makeInputMap:", err.Error())
		return args, err
	}
	_ = conv.Unmarshal(allParamStrRet, &args)
	return args, nil
}

func (t *TemplateStepList) getReturnName() string {
	returnName := ""
	for _, one := range *t {
		for _, oneStep := range one.Steps {
			onExits := strings.Split(oneStep.OnExit, "|")
			if ok, _ := cond.Contains(onExits, "return"); ok {
				returnName = oneStep.Name
				break
			}
		}
		if returnName != "" {
			break
		}
	}
	return returnName
}
func (t *TemplateStepList) getExitNameList() []string {
	exitNameList := make([]string, 0)
	for _, one := range *t {
		for _, oneStep := range one.Steps {
			onExits := strings.Split(oneStep.OnExit, "|")
			if ok, _ := cond.Contains(onExits, "exit"); ok {
				exitNameList = append(exitNameList, oneStep.Name)
			}
		}
	}
	return exitNameList
}

// Run 运行整个流程
func (t *TemplateStepList) Run(ctx workflow.Context, variable map[string]interface{}) (cmap.ConcurrentMap, error) {
	bindings := cmap.New()
	returnName := t.getReturnName()

	var err error
	for m, oneSteps := range *t {
		bindingsExecute, errExecute := t.executeAsync(ctx, oneSteps.Steps, bindings, variable)

		//重新赋值，异步执行有返回空的情况
		if bindingsExecute != nil {
			bindings = bindingsExecute
		}

		if errExecute != nil {
			//表示执行有错误了，则如果包含有returnName的话，则需要返回回去，方便获取，因为如果有错误的话，则读取不到值了
			if returnName != "" {
				if _, ok := bindings.Get(returnName); ok {
					return bindings, nil
				}
			}
			return bindings, errExecute
		}

		if returnName != "" {
			if _, ok := bindings.Get(returnName); ok {
				//后续执行一个子工作流
				var childWorkflow TemplateStepList
				for i := m + 1; i < len(*t); i++ {
					childWorkflow = append(childWorkflow, (*t)[i])
				}
				workInfo := workflow.GetInfo(ctx)
				err = workflow.ExecuteChildWorkflow(
					ctx,
					workInfo.WorkflowType.Name,
					workflow.GetActivityOptions(ctx),
					childWorkflow, variable).Get(ctx, nil)
				if err != nil {
					logs.DefaultLogger().Error("ExecuteChildWorkflow1:", err)
				}
				return bindings, nil
			}
		}
	}
	return bindings, nil
}

func (t *TemplateStepList) getOneActivityFuture(ctx workflow.Context,
	oneAct wfv1.WorkflowStep, arguments cmap.ConcurrentMap, variable map[string]interface{}) workflow.Future {
	future, settable := workflow.NewFuture(ctx)
	workflow.Go(ctx, func(ctx workflow.Context) {
		val, err := t.executeOneActivity(ctx, oneAct, arguments, variable)
		settable.Set(val, err)
	})
	return future
}

// executeAsync 执行一组异步
func (t *TemplateStepList) executeAsync(ctx workflow.Context, steps []wfv1.WorkflowStep,
	bindings cmap.ConcurrentMap, variable map[string]interface{}) (cmap.ConcurrentMap, error) {

	childCtx, cancelHandler := workflow.WithCancel(ctx)
	selector := workflow.NewSelector(ctx)
	var activityErr error
	exitNameList := t.getExitNameList()

	//这一组是否有错误退出的act
	hasExit := false
	for _, oneName := range steps {
		if ok, _ := cond.Contains(exitNameList, oneName.Name); ok {
			hasExit = true
			break
		}
	}

	for _, oneStep := range steps {
		f := t.getOneActivityFuture(childCtx, oneStep, bindings, variable)
		selector.AddFuture(f, func(f workflow.Future) {
			err := f.Get(ctx, nil)
			if err != nil {
				if hasExit {
					//如果有退出的话，则直接退出
					activityErr = err
				}
			}
		})
	}

	for i := 0; i < len(steps); i++ {
		selector.Select(ctx) // this will wait for one branch
		if activityErr != nil {
			cancelHandler()
			return bindings, activityErr
		}
	}

	return bindings, nil
}
