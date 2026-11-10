package cmd

import (
	"errors"
	"fmt"

	"fast-https/management/agentview"

	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "agent",
		Short: t("查询 AI Agent 的任务与待审批", "query AI agent tasks and pending approvals"),
		Long: t(
			"只读 Agent 的状态库，不启动 Agent，也不创建任务。\n库文件不存在时 status 为 not_running。",
			"Read the agent state database. This does not start the agent or create tasks.\nA missing database reports status not_running."),
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return usageError(fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath()))
		},
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.PersistentFlags().String("state-dir", "", t("Agent 状态目录", "agent state directory"))
	root.AddCommand(newAgentStatusCmd(), newAgentTasksCmd(), newAgentTaskCmd(), newAgentApprovalsCmd())
	return root
}

func newAgentStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Short:        t("查询 Agent 状态库", "query the agent state database"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("state-dir")
			st, err := agentview.ReadStatus(agentview.ResolveDir(dir))
			if err != nil {
				return &CLIError{Command: "agent.status", Code: CodeInternal, Message: err.Error(), Cause: err}
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "agent.status", st, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", st.Status)
			return nil
		},
	}
}

func newAgentTasksCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "tasks",
		Short:        t("列出 Agent 任务", "list agent tasks"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("state-dir")
			status, _ := cmd.Flags().GetString("status")
			tasks, err := agentview.ListTasks(agentview.ResolveDir(dir), status)
			if err != nil {
				return &CLIError{Command: "agent.tasks", Code: CodeInternal, Message: err.Error(), Cause: err}
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "agent.tasks", map[string]any{"tasks": tasks}, nil)
			}
			if len(tasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no tasks)")
				return nil
			}
			for _, t := range tasks {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", t.TaskID, t.Status, t.Type)
			}
			return nil
		},
	}
	c.Flags().String("status", "", t("按状态过滤", "filter by status"))
	return c
}

func newAgentTaskCmd() *cobra.Command {
	get := &cobra.Command{
		Use:          "get <task-id>",
		Short:        t("查看一条 Agent 任务", "show one agent task"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("state-dir")
			task, err := agentview.GetTask(agentview.ResolveDir(dir), args[0])
			if err != nil {
				code := CodeInternal
				if errors.Is(err, agentview.ErrNotFound) {
					code = CodeNotFound
				}
				return &CLIError{Command: "agent.task.get", Code: code, Message: err.Error(), Cause: err}
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "agent.task.get", task, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", task.TaskID, task.Status, task.Classification)
			return nil
		},
	}
	root := &cobra.Command{
		Use:          "task",
		Short:        t("查看 Agent 任务", "show an agent task"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(get)
	return root
}

func newAgentApprovalsCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "approvals",
		Short:        t("列出待审批计划", "list pending approvals"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("state-dir")
			items, err := agentview.ListApprovals(agentview.ResolveDir(dir))
			if err != nil {
				return &CLIError{Command: "agent.approvals", Code: CodeInternal, Message: err.Error(), Cause: err}
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "agent.approvals", map[string]any{"approvals": items}, nil)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no approvals)")
				return nil
			}
			for _, a := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", a.PlanID, a.Status, a.Reason)
			}
			return nil
		},
	}
}
