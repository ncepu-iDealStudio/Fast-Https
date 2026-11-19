// Package cacheops 是桌面端缓存区管理：全部转交主程序 cache 命令，
// 校验、原子写入与「仍被站点引用时拒绝删除」都由 CLI 负责，桌面端只做
// 参数翻译与结果文案。
package cacheops

import (
	"fmt"
	"strconv"
	"strings"

	"fast-https-desktop/internal/cli"
	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/pathutil"
)

// List 读取主配置里的缓存区。目录统一展开成绝对路径给界面显示：
// 相对写法（./cache/edge）分不清是相对程序目录还是用户目录。
func List(dir string) ([]model.CacheZone, error) {
	data, _, err := cli.RunJSON[any](dir, "cache", "list")
	if err != nil {
		return nil, err
	}
	zones := parseZones(cli.AsMap(data))
	for i := range zones {
		zones[i].Path = pathutil.Resolve(dir, zones[i].Path)
	}
	return zones, nil
}

func parseZones(data map[string]any) []model.CacheZone {
	out := make([]model.CacheZone, 0)
	for _, item := range cli.AsSlice(data["zones"]) {
		m := cli.AsMap(item)
		out = append(out, model.CacheZone{
			Name:    cli.Str(m, "name"),
			Path:    cli.Str(m, "path"),
			Valid:   cli.Strs(m, "valid"),
			MaxSize: cli.Int(m, "max_size"),
		})
	}
	return out
}

// Add 新增缓存区。界面传来的是绝对路径，写配置时收回到数据目录下的相对写法。
func Add(dir string, z model.CacheZone) (string, error) {
	if err := validate(z); err != nil {
		return "", err
	}
	z.Path = pathutil.Compact(dir, z.Path)
	args := []string{"cache", "add", "--name", strings.TrimSpace(z.Name), "--path", strings.TrimSpace(z.Path)}
	args = append(args, zoneArgs(z)...)
	saveArgs := append(args, "--max-size", strconv.Itoa(z.MaxSize))
	_, _, err := cli.RunJSON[any](dir, saveArgs...)
	if err != nil {
		return "", fmt.Errorf("%s", cli.FriendlyError(err, strings.Join(saveArgs, " ")))
	}
	return "缓存区 " + strings.TrimSpace(z.Name) + " 已创建" + note(), nil
}

// Save 修改已存在的缓存区。name 是当前名称，改名不支持（站点按名字引用）。
func Save(dir, name string, z model.CacheZone) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("缺少缓存区名")
	}
	if err := validate(z); err != nil {
		return "", err
	}
	z.Path = pathutil.Compact(dir, z.Path)
	args := []string{"cache", "update", name, "--path", strings.TrimSpace(z.Path)}
	args = append(args, zoneArgs(z)...)
	saveArgs := append(args, "--max-size", strconv.Itoa(z.MaxSize))
	_, _, err := cli.RunJSON[any](dir, saveArgs...)
	if err != nil {
		return "", fmt.Errorf("%s", cli.FriendlyError(err, strings.Join(saveArgs, " ")))
	}
	return "缓存区 " + name + " 已更新" + note(), nil
}

// Delete 删除缓存区；仍被站点引用时 CLI 会拒绝并列出引用位置。
// purge 为真时连同磁盘目录一起删除，默认只删配置。
func Delete(dir, name string, purge bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("缺少缓存区名")
	}
	args := []string{"cache", "delete", name}
	if purge {
		args = append(args, "--purge")
	}
	_, _, err := cli.RunJSON[any](dir, args...)
	if err != nil {
		return "", fmt.Errorf("%s", cli.FriendlyError(err, strings.Join(args, " ")))
	}
	if purge {
		return "缓存区 " + name + " 已删除，磁盘缓存目录已清理", nil
	}
	return "缓存区 " + name + " 已删除" + note(), nil
}

// validate 做快速校验，权威校验仍在 CLI。
func validate(z model.CacheZone) error {
	if strings.TrimSpace(z.Name) == "" {
		return fmt.Errorf("缓存区名不能为空")
	}
	if strings.TrimSpace(z.Path) == "" {
		return fmt.Errorf("缓存目录不能为空")
	}
	if z.MaxSize < 0 {
		return fmt.Errorf("最大容量不能为负数")
	}
	hasValid := false
	for _, v := range z.Valid {
		if strings.TrimSpace(v) != "" {
			hasValid = true
			break
		}
	}
	if !hasValid {
		return fmt.Errorf("有效期不能为空：没有 valid 规则时缓存区不会缓存任何响应")
	}
	return nil
}

// zoneArgs 生成有效期参数。空列表显式传 --valid=，
// 这样 update 能真的清空，而不是「没写 = 不改」。
func zoneArgs(z model.CacheZone) []string {
	var args []string
	for _, v := range z.Valid {
		if s := strings.TrimSpace(v); s != "" {
			args = append(args, "--valid", s)
		}
	}
	if len(args) == 0 {
		return []string{"--valid="}
	}
	return args
}

func note() string {
	return "；下次 reload 或启动时生效"
}
