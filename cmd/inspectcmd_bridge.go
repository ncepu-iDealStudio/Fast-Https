package cmd

import (
	"fast-https/cmd/internal/inspectcmd"

	"github.com/spf13/cobra"
)

// 只读观测类命令已迁到 cmd/internal/inspectcmd。这里保留 cmd 包内的装配
// 入口，供 commands.go 注册到根命令树。

func newRuntimeCmd() *cobra.Command { return inspectcmd.NewRuntimeCmd() }

func newMonitorCmd() *cobra.Command { return inspectcmd.NewMonitorCmd() }

func newLogsCmd() *cobra.Command { return inspectcmd.NewLogsCmd() }

func newCertCmd() *cobra.Command { return inspectcmd.NewCertCmd() }

func newDiagnoseCmd() *cobra.Command { return inspectcmd.NewDiagnoseCmd() }

func newOperationCmd() *cobra.Command { return inspectcmd.NewOperationCmd() }
