package view

import (
	"os"

	"fast-https/config"
)

// ConfigValidateData 是校验通过时的数据。校验失败不在这里表达：
// 返回 InvalidError，由 cmd 转成 CONFIG_INVALID 错误封套。
type ConfigValidateData struct {
	Valid     bool   `json:"valid"`
	File      string `json:"file"`
	Candidate bool   `json:"candidate"`
}

// ConfigValidate 校验一个主配置文件及其 include。candidate 为真表示这是用户给的候选文件。
// 注意此阶段 include 与证书路径仍按运行期基址解析，所以候选只覆盖「主配置文件」本身。
func ConfigValidate(path string, candidate bool) (ConfigValidateData, []string, error) {
	data := ConfigValidateData{File: path, Candidate: candidate}
	if _, err := os.Stat(path); err != nil {
		return data, nil, err
	}

	warnings, err := config.ValidateConfigFileWithWarnings(path)
	if err != nil {
		return data, nil, &InvalidError{Message: err.Error()}
	}
	if candidate {
		warnings = append(warnings,
			"candidate scope is main file only: http.include and certificate paths are resolved against the running layout")
	}
	data.Valid = true
	return data, warnings, nil
}
