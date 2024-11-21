package activity

import (
	"context"
	"fmt"
	"github.com/tianlin0/plat-lib/conv"
	"github.com/tianlin0/plat-lib/curl"
	"github.com/tianlin0/plat-lib/goroutines"
	"github.com/tianlin0/plat-lib/logs"
	"github.com/tianlin0/plat-lib/utils"
	"github.com/tianlin0/plat-lib/utils/httputil"
	"go.temporal.io/api/workflowservice/v1"
	temporalActivity "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

type (
	TemplateMethod func(ctx context.Context, param map[string]interface{}) (map[string]interface{}, error)
	// TemplateActivity 编写 Activity 的单独模式
	TemplateActivity interface {
		Template() string          //定义的使用名字
		GetMethod() TemplateMethod //返回的方法
	}
)

const (
	Variables = "variables" //传入的总变量名
	Arguments = "arguments"
	Responses = "responses"
	Result    = "result" //返回值默认的key
)

var (
	templateActivityFuncMap = sync.Map{}
)

type commActivity struct {
}

// New 新建
func New() *commActivity {
	return new(commActivity)
}

func (a *commActivity) GetActivityName(taskQueueName string, name string) string {
	return fmt.Sprintf("%s/%s", taskQueueName, name)
}
func (a *commActivity) GetAllActivityList(taskQueueName string) []string {
	actList := make([]string, 0)
	templateActivityFuncMap.Range(func(key, value any) bool {
		keyStr := conv.String(key)
		if strings.HasPrefix(keyStr, taskQueueName+"/") {
			actList = utils.AppendUnique(actList, keyStr)
		}
		return true
	})
	return actList
}

// GetActivityMethodFunc 获取activity名称
func (a *commActivity) GetActivityMethodFunc(taskQueueName string, name string) interface{} {
	storeName := a.GetActivityName(taskQueueName, name)

	if methodFunc, ok := templateActivityFuncMap.Load(storeName); ok {
		return methodFunc
	}
	return nil
}

// GetActivityMethodArguments 获取activity参数列表
func (a *commActivity) GetActivityMethodArguments(taskQueueName string,
	name string, oldParam []interface{}) ([]interface{}, error) {
	funcTypeList := a.getActivityArgumentTypes(taskQueueName, name)

	var err error

	//判断是否是变长参数
	var isVariadic = false
	var funcTypeLen = len(funcTypeList)
	if funcTypeLen > 0 {
		lastParam := funcTypeList[funcTypeLen-1]
		if lastParam.Kind() == reflect.Slice {
			isVariadic = true
		}
	}

	// 遍历函数的参数列表
	paramList := make([]interface{}, 0)
	m := 0
	for i := 0; i < funcTypeLen; i++ {
		paramType := funcTypeList[i]

		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}

		if m < len(oldParam) {
			oldParamTemp := oldParam[m]
			m++

			if reflect.TypeOf(oldParamTemp) == paramType {
				paramList = append(paramList, oldParamTemp)
				continue
			}

			// 如果是变长参数
			if i == funcTypeLen-1 && isVariadic {
				newVariadicPtr := reflect.ValueOf(conv.NewPtrByType(paramType))
				newVariadicList := newVariadicPtr.Elem()
				//往后退一位
				for m = m - 1; m < len(oldParam); m++ {
					oldParamTemp = oldParam[m]
					if reflect.TypeOf(oldParamTemp) == paramType.Elem() {
						newVariadicList = reflect.Append(newVariadicList, reflect.ValueOf(oldParamTemp))
					}
				}
				paramList = append(paramList, newVariadicList.Interface())
				continue
			}

			// 进行转换
			dstIn := conv.NewPtrByType(paramType)
			errTemp := conv.AssignTo(oldParamTemp, dstIn)
			if errTemp == nil {
				if reflect.TypeOf(dstIn) == paramType {
					paramList = append(paramList, dstIn)
					continue
				} else {
					if reflect.TypeOf(dstIn).Elem() == paramType {
						paramList = append(paramList, reflect.ValueOf(dstIn).Elem().Interface())
						continue
					}
				}
			}

			// 默认设置
			dst := reflect.New(paramType)
			errTemp = conv.AssignTo(oldParamTemp, dst)
			if errTemp != nil {
				err = errTemp
				logs.DefaultLogger().Error(errTemp, paramType.String())
			}
			paramList = append(paramList, dst.Elem().Interface())
			continue
		}

		//添加默认值
		dst := reflect.New(paramType)
		paramList = append(paramList, dst.Elem().Interface())
	}

	// 配置变量的参数加上
	for k := m; k < len(oldParam); k++ {
		paramList = append(paramList, oldParam[k])
	}

	return paramList, err
}

// getActivityArgumentTypes 获取activity参数列表的类型
func (a *commActivity) getActivityArgumentTypes(taskQueueName string, name string) []reflect.Type {
	methodFun := a.GetActivityMethodFunc(taskQueueName, name)

	funcType := reflect.TypeOf(methodFun)

	// 遍历函数的参数列表
	paramList := make([]reflect.Type, 0)
	for i := 0; i < funcType.NumIn(); i++ {
		paramType := funcType.In(i)
		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() {
			continue
		}
		paramList = append(paramList, paramType)
	}

	return paramList
}

// getActivityMethodOutputs 获取activity返回列表
func (a *commActivity) getActivityMethodOutputs(taskQueueName string, name string) ([]interface{}, error) {
	methodFun := a.GetActivityMethodFunc(taskQueueName, name)

	funcType := reflect.TypeOf(methodFun)

	var err error

	// 遍历函数的参数列表
	outputList := make([]interface{}, 0)
	for i := 0; i < funcType.NumOut(); i++ {
		paramType := funcType.Out(i)

		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() ||
			paramType == reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}

		retData := conv.NewPtrByType(paramType)
		if retData != nil {
			outputList = append(outputList, retData)
		} else {
			err = fmt.Errorf("NewPtrByType error: %s", paramType.String())
		}
	}
	return outputList, err
}

// GetActivityMethodOutput 获取activity返回的可用的指针对象，默认为map[string]interface{}
func (a *commActivity) GetActivityMethodOutput(taskQueueName string, name string) (interface{}, error) {
	defaultRet := &map[string]interface{}{}

	list, err := a.getActivityMethodOutputs(taskQueueName, name)
	if err != nil {
		return defaultRet, err
	}

	if list != nil && len(list) > 0 {
		return list[0], nil
	}

	return defaultRet, nil
}

// SetActivityMethodName 设置activity的方法名
func (a *commActivity) SetActivityMethodName(taskQueueName string, name string, methodFun interface{}) error {
	if name == "" {
		logs.DefaultLogger().Error("name is empty")
		return fmt.Errorf("name is empty: %s", name)
	}
	storeName := a.GetActivityName(taskQueueName, name)

	if _, ok := templateActivityFuncMap.Load(storeName); ok {
		logs.DefaultLogger().Error("name has store", name)
		return fmt.Errorf("name %s has stored", name)
	}
	templateActivityFuncMap.Store(storeName, methodFun)
	return nil
}

// CurlActivityExecute http请求
func CurlActivityExecute(ctx context.Context, curlReq *curl.Request) (*curl.Response, error) {
	if curlReq == nil {
		return nil, fmt.Errorf("data error")
	}

	var resp *curl.Response
	var err error
	goroutines.GoSyncHandler(func(params ...interface{}) {
		logger := logs.DefaultLogger()
		logger.Info("CurlActivityExecute begin:", conv.String(curlReq))
		resp = curl.NewRequest(curlReq).Submit(ctx)
		logger.Info("CurlActivityExecute end:", resp.Error, resp.HttpStatus, resp.Response)

		if resp.HttpStatus != http.StatusOK {
			if resp.Response != "" {
				nResp := &httputil.CommResponse{}
				_ = conv.Unmarshal(resp.Response, nResp)
				if nResp.Message != "" {
					err = fmt.Errorf(nResp.Message)
				}
			}
			if err == nil {
				err = fmt.Errorf("return not StatusOk: %d, %v", resp.HttpStatus, resp.Error)
			}
		}
		if resp.Error != nil {
			if err == nil {
				err = resp.Error
			}
		}
	}, nil, ctx)
	return resp, err
}

func (a *commActivity) GetErrorByTemporalError(err error) error {
	if err == nil {
		return nil
	}
	strMsg := err.Error()
	//activity error (type: gdp-dsl/swoole-php/add-paas, scheduledEventID: 5,
	//startedEventID: 6, identity: 1@gdp-appserver-go-dev-v3-846dd7d9bb-dgj74@):
	//在该项目(dffds): 下英文名称[fdsfdf2223-gdpdev]已经存在了
	re := regexp.MustCompile(`[^\(]*\([^\)]+\): (.*)$`)
	match := re.FindStringSubmatch(strMsg)
	if len(match) > 1 {
		errorMessage := match[1]
		return fmt.Errorf(errorMessage)
	}
	//该项目还没有申请到可用资源:cls-3bql65fd (type: derek-proj-gateway-gdppre.dsfdsfsd-gdppre, retryable: true)
	//未定义的错误码 (type: derek-proj-gateway-gdppre.erere-gdppre, retryable: true)
	//(type: derek-proj-gateway-gdppre.erere-gdppre, retryable: true)
	re = regexp.MustCompile(`([^\(]+)\(type:.+, retryable: (true|false)\)$`)
	match = re.FindStringSubmatch(strMsg)
	if len(match) > 1 {
		errorMessage := match[1]
		return fmt.Errorf(errorMessage)
	}

	return err
}

func (a *commActivity) GetWorkflowInfo(ctx context.Context, temporalClient client.Client) (temporalActivity.Info, *workflowservice.DescribeWorkflowExecutionResponse) {
	activityInfo := temporalActivity.GetInfo(ctx)
	if temporalClient == nil {
		return activityInfo, nil
	}
	exeResponse, err := temporalClient.DescribeWorkflowExecution(ctx, activityInfo.WorkflowExecution.ID, activityInfo.WorkflowExecution.RunID)
	if err != nil {
		return activityInfo, nil
	}
	return activityInfo, exeResponse
}
