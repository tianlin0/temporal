package workflow

import (
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tianlin0/plat-lib/cond"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/templates"
	"github.com/tianlin0/temporal/activity"
	"regexp"
)

type commWorkflow struct {
}

func New() *commWorkflow {
	return new(commWorkflow)
}

func (t *commWorkflow) ShouldExecute(when string, inputMap ...interface{}) (bool, error) {
	if when == "" {
		return true, nil
	}
	expReal := templates.NewTemplate(when)
	when, err := expReal.Replace(inputMap...)
	if err != nil {
		return false, err
	}
	result, err := templates.ExpressEvaluateFromToken(when)
	if err != nil {
		return false, err
	}
	boolRes, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expected boolean evaluation for '%s'. Got %v", when, result)
	}
	return boolRes, nil
}

func (t *commWorkflow) ChangeConcurrentMapToMap(arguments cmap.ConcurrentMap) map[string]interface{} {
	args := make(map[string]interface{})
	if arguments != nil && !arguments.IsEmpty() {
		keys := arguments.Keys()
		for _, key := range keys {
			args[key], _ = arguments.Get(key)
		}
	}
	return args
}

func (a *commWorkflow) ExtendToBindings(bindings cmap.ConcurrentMap, inputOrReturn map[string]interface{}, id string, position string) (cmap.ConcurrentMap, error) {
	if inputOrReturn == nil || len(inputOrReturn) == 0 {
		return bindings, nil
	}
	if id == "" {
		return bindings, fmt.Errorf("id is null")
	}

	defaultMap := map[string]interface{}{
		activity.Arguments: map[string]interface{}{},
		activity.Responses: map[string]interface{}{},
	}

	formatTrue := true
	if iData, ok := bindings.Get(id); !ok {
		formatTrue = false
	} else {
		if _, ok := iData.(map[string]interface{}); !ok {
			formatTrue = false
		}
	}

	if !formatTrue {
		bindings.Set(id, defaultMap)
	}

	idData, _ := bindings.Get(id)

	var tempAllBindings map[string]interface{}
	if tempBindings, ok := idData.(map[string]interface{}); !ok {
		return bindings, fmt.Errorf("id format error:" + conv.String(idData))
	} else {
		tempAllBindings = tempBindings
	}

	if _, ok := tempAllBindings[position]; !ok {
		tempAllBindings[position] = map[string]interface{}{}
	}
	if inputOrReturn == nil || len(inputOrReturn) == 0 {
		delete(tempAllBindings, position)
	} else {
		tempAllBindings[position] = inputOrReturn
	}
	bindings.Set(id, tempAllBindings)
	return bindings, nil
}

func (a *commWorkflow) CheckArguments(id string, inputParam interface{}) error {
	regInfo, err := regexp.Compile("\\{\\{[^\\}]+\\}\\}")
	if err == nil {
		inputStr := conv.String(inputParam)
		paramCheck := regInfo.FindAllString(inputStr, -1)
		if len(paramCheck) > 0 {
			return fmt.Errorf("参数获取失败：%s, %s", id, inputStr)
		}
	}
	return nil
}

// GetInputMap 获取输入参数
func (t *commWorkflow) GetInputMap(id string, arguments cmap.ConcurrentMap, variable map[string]interface{}, allWorkflowId []string) map[string]interface{} {
	args := make(map[string]interface{})

	// 全局传进来的参数
	if variable != nil || len(variable) > 0 {
		//1、首先将不是id的参数都添加进来
		for k, v := range variable {
			if allWorkflowId != nil && len(allWorkflowId) > 0 {
				if ok, _ := cond.Contains(allWorkflowId, k); ok {
					continue
				}
			}
			args[k] = v
		}
		//2、将是自己id的参数覆盖进来
		if id != "" {
			if oneParam, ok := variable[id]; ok {
				if oneParamMap, ok1 := oneParam.(map[string]interface{}); ok1 {
					for k, v := range oneParamMap {
						args[k] = v
					}
				}
			}
		}
	}

	// 本身的参数列表是否包含
	//3、activity中自定义进行覆盖，主要是将前面流程的参数和返回值加到里面
	comm := New()
	newArgs := comm.ChangeConcurrentMapToMap(arguments)
	for key, val := range newArgs {
		args[key] = val
	}

	return args
}

func (t *commWorkflow) ReplaceAllByBindings(args interface{}, bindings cmap.ConcurrentMap) error {
	bindingsMap := t.ChangeConcurrentMapToMap(bindings)

	argsMapList := conv.ToKeyListFromMap(bindingsMap)
	allParamStr := conv.String(args)
	tmp := templates.NewTemplate(allParamStr)
	allParamStrRet, err := tmp.Replace(argsMapList)
	if err != nil {
		logs.DefaultLogger().Error("ReplaceAllByBindings:", err)
		return err
	}
	_ = conv.Unmarshal(allParamStrRet, args)
	return nil
}

// GetOutputMap 获取自定义输出参数
func (t *commWorkflow) GetOutputMap(defineMap map[string]interface{}, allOutMap map[string]interface{}, bindings cmap.ConcurrentMap) (map[string]interface{}, error) {
	args := make(map[string]interface{})

	//全部所有返回值
	for k, v := range allOutMap {
		args[k] = v
	}

	//自定义的值进行覆盖，为空的话，则跳过
	for key, arg := range defineMap {
		if conv.String(arg) != "" {
			args[key] = arg
		}
	}

	tmp := templates.NewTemplate(conv.String(args))
	allParamStrRet, err := tmp.Replace(bindings)
	if err != nil {
		logs.DefaultLogger().Error("makeOutputMap:", err.Error())
		return args, err
	}

	logs.DefaultLogger().Info(allParamStrRet)

	err = conv.Unmarshal(allParamStrRet, &args)
	return args, err
}

func (t *commWorkflow) GetDslWorkflow() *dslWorkflow {
	return new(dslWorkflow)
}
