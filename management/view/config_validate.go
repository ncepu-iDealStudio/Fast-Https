package view

import (
	"errors"
	"io/fs"
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

// ConfigValidate 校验当前配置或一棵候选目录树。
// candidate 为真时 path 是候选目录（或其中的 fast-https.json），include 与证书按该树解析。
func ConfigValidate(path string, candidate bool) (ConfigValidateData, []string, error) {
	data := ConfigValidateData{File: path, Candidate: candidate}
	if candidate {
		root, err := config.ResolveCandidateRoot(path)
		if err != nil {
			return data, nil, err
		}
		data.File = root
		warnings, err := config.ValidateConfigTree(root)
		if err != nil {
			if errors.Is(err, config.ErrCandidatePath) || errors.Is(err, fs.ErrNotExist) {
				return data, nil, err
			}
			return data, nil, &InvalidError{Message: err.Error()}
		}
		data.Valid = true
		return data, warnings, nil
	}

	if _, err := os.Stat(path); err != nil {
		return data, nil, err
	}

	warnings, err := config.ValidateConfigFileWithWarnings(path)
	if err != nil {
		return data, nil, &InvalidError{Message: err.Error()}
	}
	data.Valid = true
	return data, warnings, nil
}
