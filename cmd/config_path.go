package cmd

import (
	"encoding/json"
	"fast-https/config"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// config 命令组给外部控制面一个稳定的取值入口：控制面是独立进程，
// 不能靠猜"有没有装成包"来决定路径。归档与 deb/rpm/apk 的布局不同，
// 由主程序按编译期基址算好后输出，控制面直接读即可。

// RuntimePaths 是一次路径查询的结果，四个字段都是绝对路径。
type RuntimePaths struct {
	Config string `json:"config"`
	Cert   string `json:"cert"`
	Log    string `json:"log"`
	Pid    string `json:"pid"`
}

func newConfigCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "config",
		Short:        "inspect resolved runtime paths",
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	var asJSON bool
	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "print config, cert, log and pid paths (absolute)",
		Long: "print config, cert, log and pid paths (absolute).\n\n" +
			"log 是当前布局的默认日志目录；配置里写了 log_root 时以配置为准，\n" +
			"控制面可读取配置文件同名字段覆盖本值。",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := ResolvePaths()
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(paths)
			}
			fmt.Printf("config %s\n", paths.Config)
			fmt.Printf("cert   %s\n", paths.Cert)
			fmt.Printf("log    %s\n", paths.Log)
			fmt.Printf("pid    %s\n", paths.Pid)
			return nil
		},
	}
	pathCmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	root.AddCommand(pathCmd)
	return root
}

// ResolvePaths 返回当前编译布局下的绝对路径。
func ResolvePaths() RuntimePaths {
	return RuntimePaths{
		Config: toAbs(config.CONFIG_FILE_PATH),
		Cert:   toAbs(config.CERT_DIR),
		Log:    toAbs(config.DEFAULT_LOG_ROOT),
		Pid:    toAbs(config.PID_FILE),
	}
}

// toAbs 把归档布局的相对路径固定到当前工作目录：输出一律是绝对路径，
// 控制面与 monitor 不需要再自己拼。
func toAbs(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
