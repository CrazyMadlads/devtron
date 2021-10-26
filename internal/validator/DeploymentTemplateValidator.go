package DeploymentTemplateValidate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/devtron-labs/devtron/internal/util"
	util2 "github.com/devtron-labs/devtron/util"
	"github.com/xeipuuv/gojsonschema"
)

type (
	CpuChecker    struct{}
	MemoryChecker struct{}
)

var (
	CpuUnitChecker, _   = regexp.Compile("^([0-9.]+)m$")
	NoCpuUnitChecker, _ = regexp.Compile("^([0-9.]+)$")
	MiChecker, _  = regexp.Compile("^[0-9]+Mi$")
	GiChecker, _  = regexp.Compile("^[0-9]+Gi$")
	TiChecker, _  = regexp.Compile("^[0-9]+Ti$")
	PiChecker, _  = regexp.Compile("^[0-9]+Pi$")
	KiChecker, _  = regexp.Compile("^[0-9]+Ki$")
)

func (f CpuChecker) IsFormat(input interface{}) bool {
	asString, ok := input.(string)
	if !ok {
		return true
	}

	if CpuUnitChecker.MatchString(asString) {
		return true
	} else if NoCpuUnitChecker.MatchString(asString) {
		return true
	} else {
		return false
	}
}

func (f MemoryChecker) IsFormat(input interface{}) bool {
	asString, ok := input.(string)
	if !ok {
		return true
	}

	if MiChecker.MatchString(asString) {
		return true
	} else if GiChecker.MatchString(asString) {
		return true
	} else if TiChecker.MatchString(asString) {
		return true
	} else if PiChecker.MatchString(asString) {
		return true
	} else if KiChecker.MatchString(asString) {
		return true
	} else {
		return false
	}
}

const memoryPattern = `"100Mi" or "1Gi" or "1Ti"`
const cpuPattern = `"50m" or "0.05"`
const cpu = "cpu"
const memory = "memory"

func DeploymentTemplateValidate(templatejson interface{}, schemafile string) (bool, error) {

	sugaredLogger := util.NewSugardLogger()
	if _, err := os.Stat(fmt.Sprintf("schema/%s.json", schemafile)); err == nil {
		gojsonschema.FormatCheckers.Add("cpu", CpuChecker{})
		gojsonschema.FormatCheckers.Add("memory", MemoryChecker{})

		jsonFile, err := os.Open(fmt.Sprintf("schema/%s.json", schemafile))
		if err != nil {
			sugaredLogger.Errorw("scheme file error", err)
		}
		byteValueJsonFile, _ := ioutil.ReadAll(jsonFile)
		var schemajson map[string]interface{}
		json.Unmarshal([]byte(byteValueJsonFile), &schemajson)
		schemaLoader := gojsonschema.NewGoLoader(schemajson)
		documentLoader := gojsonschema.NewGoLoader(templatejson)
		marshalTemplatejson, err := json.Marshal(templatejson)
		if err != nil {
			sugaredLogger.Errorw("Error in marshalling templatejson",err)
			return false, err
		}
		result, err := gojsonschema.Validate(schemaLoader, documentLoader)
		if err != nil {
			sugaredLogger.Errorw("Error in gojsonschema validation",err)
			return false, err
		}
		if result.Valid() {
			var dat map[string]interface{}

			if err := json.Unmarshal(marshalTemplatejson, &dat); err != nil {
				sugaredLogger.Errorw("Error in unmarshal templatejson",err)
				return false, err
			}
			//limits and requests are mandatory fields in schema
			autoscaleEnabled,ok := dat["autoscaling"].(map[string]interface{})["enabled"]
			if ok && autoscaleEnabled.(bool) {
				checkCPUlimit,ok := dat["resources"].(map[string]interface{})["limits"].(map[string]interface{})["cpu"]
				if !ok{
					return false,errors.New("CPU limit is required")
				}
				checkMemorylimit,ok := dat["resources"].(map[string]interface{})["limits"].(map[string]interface{})["memory"]
				if !ok{
					return false,errors.New("Memory limit is required")
				}
				checkCPURequests,ok := dat["resources"].(map[string]interface{})["requests"].(map[string]interface{})["cpu"]
				if !ok{
					return false,errors.New("CPU requests is required")
				}
				checkMemoryRequests,ok := dat["resources"].(map[string]interface{})["requests"].(map[string]interface{})["memory"]
				if !ok{
					return false,errors.New("Memory requests is required")
				}
				checkEnvoproxyCPUlimit,ok := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["limits"].(map[string]interface{})["cpu"]
				if !ok{
					return false,errors.New("Envoproxy CPU limit is required")
				}
				checkEnvoproxyMemorylimit,ok := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["limits"].(map[string]interface{})["memory"]
				if !ok{
					return false,errors.New("Envoproxy Memory limit is required")
				}
				checkEnvoproxyCPURequests,ok := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["requests"].(map[string]interface{})["cpu"]
				if !ok{
					return false,errors.New("Envoproxy CPU requests is required")
				}
				checkEnvoproxyMemoryRequests,ok := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["requests"].(map[string]interface{})["memory"]
				if !ok{
					return false,errors.New("Envoproxy memory requests is required")
				}
				//limit := dat["resources"].(map[string]interface{})["limits"].(map[string]interface{})
				//request := dat["resources"].(map[string]interface{})["requests"].(map[string]interface{})

				cpu_limit, _ := util2.CpuToNumber(checkCPUlimit.(string))
				memory_limit, _ := util2.MemoryToNumber(checkMemorylimit.(string))
				cpu_request, _ := util2.CpuToNumber(checkCPURequests.(string))
				memory_request, _ := util2.MemoryToNumber(checkMemoryRequests.(string))

				//envoproxy_limit := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["limits"].(map[string]interface{})
				//envoproxy_request := dat["envoyproxy"].(map[string]interface{})["resources"].(map[string]interface{})["requests"].(map[string]interface{})

				envoproxy_cpu_limit, _ := util2.CpuToNumber(checkEnvoproxyCPUlimit.(string))
				envoproxy_memory_limit, _ := util2.MemoryToNumber(checkEnvoproxyMemorylimit.(string))
				envoproxy_cpu_request, _ := util2.CpuToNumber(checkEnvoproxyCPURequests.(string))
				envoproxy_memory_request, _ := util2.MemoryToNumber(checkEnvoproxyMemoryRequests.(string))
				if (envoproxy_cpu_limit < envoproxy_cpu_request) || (envoproxy_memory_limit < envoproxy_memory_request) || (cpu_limit < cpu_request) || (memory_limit < memory_request) {
					return false, errors.New("requests is greater than limits")
				}

			}

			return true, nil
		} else {
			var stringerror string
			for _, err := range result.Errors() {
				fmt.Println(err.Details()["format"])
				if err.Details()["format"] == cpu {
					stringerror = stringerror + err.Field() + ": Format should be like " + cpuPattern + "\n"
				} else if err.Details()["format"] == memory {
					stringerror = stringerror + err.Field() + ": Format should be like " + memoryPattern + "\n"
				} else {
					stringerror = stringerror + err.String() + "\n"
				}
			}
			return false, errors.New(stringerror)
		}
	}else {
		return true,nil
	}
}