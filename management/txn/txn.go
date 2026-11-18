// Package txn 实现配置事务：恢复点、切换、reload、健康验证、补偿恢复、幂等。
package txn

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"fast-https/config"
	"fast-https/management/cfgdiff"
	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/state"
	"fast-https/management/svcctl"
)

const (
	PhaseStarted       = "started"
	PhaseValidated     = "validated"
	PhaseBackedUp      = "backup_saved"
	PhaseFilesSwitched = "files_switched"
	PhaseReloaded      = "reloaded"
	PhaseVerified      = "verified"
	PhaseCommitted     = "committed"
	PhaseCompensating  = "compensating"
	PhaseRestored      = "restored"
	PhaseRestoreVerified = "restore_verified"

	StateMatchesCandidate = "matches_candidate"
	StateMatchesPrevious  = "matches_previous"
	StateUnchanged        = "unchanged"
	StateUncertain        = "uncertain"
)

var errStopped = errors.New("engine stopped after phase (test)")

// Reloader 发 reload 信号。测试注入失败。
type Reloader interface {
	Reload(ctx context.Context) error
}

// HealthChecker 做弱验证。测试注入失败。
type HealthChecker interface {
	Check(ctx context.Context) error
}

// FuncReloader 把函数做成 Reloader。
type FuncReloader func(ctx context.Context) error

func (f FuncReloader) Reload(ctx context.Context) error { return f(ctx) }

// FuncHealth 把函数做成 HealthChecker。
type FuncHealth func(ctx context.Context) error

func (f FuncHealth) Check(ctx context.Context) error { return f(ctx) }

// Input 是一次 apply/rollback。
type Input struct {
	Candidate    string
	LiveMain     string
	RequestID    string
	Command      string
	Summary      string
	DryRun       bool
	RestoreCerts bool
	Args         map[string]string
}

// Result 是事务结果。
type Result struct {
	Operation   operation.Record
	Diff        cfgdiff.Result
	Version     string
	RolledBack  bool
	ConfigState string
	DryRun      bool
	Idempotent  bool
	Warnings    []string
}

// Engine 跑配置事务。StopAfter / RestoreErr 仅测试使用。
type Engine struct {
	State      state.Dir
	Reloader   Reloader
	Health     HealthChecker
	Files      FileOps
	CertDir    string
	Probe      func(int) bool
	Bind       func(int) error
	StopAfter  string
	RestoreErr error
}

func (e *Engine) certDir() string {
	if e.CertDir != "" {
		return e.CertDir
	}
	return config.CERT_DIR
}

func (e *Engine) ops() operation.Store { return operation.Store{Dir: e.State} }

func (e *Engine) reloader() Reloader {
	if e.Reloader != nil {
		return e.Reloader
	}
	return FuncReloader(func(context.Context) error { return nil })
}

func (e *Engine) health() HealthChecker {
	if e.Health != nil {
		return e.Health
	}
	return FuncHealth(func(context.Context) error { return nil })
}

// Apply 按规范 §5.1 执行。dry-run 只校验和差异，不写正式文件、不 reload、不建恢复点。
func (e *Engine) Apply(ctx context.Context, in Input) (Result, error) {
	restore := ignoreHangup()
	defer restore()

	if in.Command == "" {
		in.Command = "config.apply"
	}
	candRoot, err := config.ResolveCandidateRoot(in.Candidate)
	if err != nil {
		return Result{}, wrap(CodeUsage, err.Error(), err)
	}
	if in.LiveMain == "" {
		in.LiveMain = config.CONFIG_FILE_PATH
	}
	liveMain, err := filepath.Abs(in.LiveMain)
	if err != nil {
		return Result{}, wrap(CodeFailed, err.Error(), err)
	}

	if !in.DryRun {
		if err := e.checkWritable(liveMain); err != nil {
			return Result{}, wrap(CodePermission, err.Error(), err)
		}
	}

	// 候选树只带 json，相对证书路径要以**线上配置目录**为基准解析：
	// 桌面端从快捷方式启动时进程 CWD 未必是安装目录，靠 CWD 会误判成证书缺失。
	warnings, err := config.ValidateConfigTree(candRoot, filepath.Dir(liveMain))
	if err != nil {
		if errors.Is(err, config.ErrCandidatePath) {
			return Result{}, wrap(CodeUsage, err.Error(), err)
		}
		if os.IsNotExist(err) {
			return Result{}, wrap(CodeNotFound, err.Error(), err)
		}
		return Result{}, wrap(CodeConfigInvalid, err.Error(), err)
	}

	liveRoot := filepath.Dir(liveMain)
	diff, err := cfgdiff.Trees(liveRoot, candRoot)
	if err != nil {
		return Result{}, wrap(CodeFailed, err.Error(), err)
	}

	out := Result{Diff: diff, Warnings: warnings, ConfigState: StateUnchanged}

	if in.DryRun {
		out.DryRun = true
		return out, nil
	}

	if _, err := svcctl.PrecheckTree(candRoot, e.Probe, e.Bind); err != nil {
		return Result{}, asTxn(err)
	}

	args := in.Args
	if args == nil {
		args = map[string]string{"candidate": candRoot, "live": liveMain}
	}
	rec, err := e.ops().Begin(operation.BeginInput{
		RequestID: in.RequestID,
		Command:   in.Command,
		Args:      args,
		Phase:     PhaseStarted,
	})
	if err != nil {
		return Result{}, asTxn(err)
	}
	out.Operation = rec
	if rec.Status != operation.StatusRunning {
		out.Idempotent = true
		out.ConfigState = resultState(rec)
		out.Version, _ = rec.Result["version"].(string)
		out.RolledBack, _ = rec.Result["rolled_back"].(bool)
		return out, nil
	}
	if rec.Phase != PhaseStarted {
		out.Idempotent = true
		out.ConfigState = StateUncertain
		return out, nil
	}

	lock, err := e.State.Lock()
	if err != nil {
		return out, attachOp(asTxn(err), rec)
	}
	defer lock()

	if err := e.setPhase(&rec, PhaseValidated); err != nil {
		return out, attachOp(err, rec)
	}
	if e.stop(PhaseValidated) {
		out.Operation = rec
		return out, errStopped
	}

	// 备份只用于失败补偿，放系统临时目录，成功即删；不回滚历史。
	backup, err := os.MkdirTemp("", "fast-https-backup-")
	if err != nil {
		return out, e.fail(rec, StateUnchanged, err)
	}
	defer os.RemoveAll(backup)
	if err := e.backupTree(liveMain, backup); err != nil {
		return out, e.fail(rec, StateUnchanged, err)
	}
	if in.RestoreCerts {
		if err := cfgver.BackupCerts(e.certDir(), filepath.Join(backup, "certs")); err != nil {
			return out, e.fail(rec, StateUnchanged, err)
		}
	}
	// 第一次操作时给「操作前」的配置留一个基线版本（v001），
	// 之后每次成功操作只保存结果快照；它不是每次操作的恢复点。
	if !cfgver.HasSnapshots(e.State) {
		if _, err := cfgver.SnapshotWithCerts(e.State, liveMain, e.certDir(), "initial "+in.Command, e.State.CurrentTime()); err != nil {
			return out, e.fail(rec, StateUnchanged, err)
		}
	}
	if err := e.setPhase(&rec, PhaseBackedUp); err != nil {
		return out, attachOp(err, rec)
	}
	if e.stop(PhaseBackedUp) {
		out.Operation = rec
		return out, errStopped
	}

	if err := e.switchFiles(liveMain, candRoot); err != nil {
		return out, e.compensate(ctx, rec, liveMain, backup, err)
	}
	if err := e.setPhase(&rec, PhaseFilesSwitched); err != nil {
		return out, attachOp(err, rec)
	}
	if e.stop(PhaseFilesSwitched) {
		out.Operation = rec
		return out, errStopped
	}

	var certs []string
	if in.RestoreCerts {
		certs, err = cfgver.RestoreCerts(candRoot, e.certDir())
		if err != nil {
			return out, e.compensate(ctx, rec, liveMain, backup, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return out, e.unknown(rec, err)
	}
	if err := e.reloader().Reload(ctx); err != nil {
		return out, e.compensate(ctx, rec, liveMain, backup, err)
	}
	if err := e.setPhase(&rec, PhaseReloaded); err != nil {
		return out, attachOp(err, rec)
	}
	if e.stop(PhaseReloaded) {
		out.Operation = rec
		return out, errStopped
	}

	if err := e.health().Check(ctx); err != nil {
		return out, e.compensate(ctx, rec, liveMain, backup, err)
	}
	if err := e.setPhase(&rec, PhaseVerified); err != nil {
		return out, attachOp(err, rec)
	}
	if e.stop(PhaseVerified) {
		out.Operation = rec
		return out, errStopped
	}

	man, err := cfgver.SnapshotWithCerts(e.State, liveMain, e.certDir(), in.Summary, e.State.CurrentTime())
	if err != nil {
		return out, e.fail(rec, StateMatchesCandidate, err)
	}
	out.Version = man.Version
	out.ConfigState = StateMatchesCandidate
	result := map[string]any{
		"version":      man.Version,
		"rolled_back":  false,
		"config_state": StateMatchesCandidate,
		"changes":      out.Diff.Changes,
	}
	if len(certs) > 0 {
		result["certs_restored"] = certs
	}
	rec, err = e.ops().Finish(rec, operation.StatusSucceeded, PhaseCommitted, result, nil)
	if err != nil {
		return out, attachOp(err, rec)
	}
	out.Operation = rec
	return out, nil
}

func (e *Engine) checkWritable(liveMain string) error {
	fsys := e.files()
	if err := fsys.Writable(filepath.Dir(liveMain)); err != nil {
		return err
	}
	return fsys.Writable(e.State.Root)
}

func (e *Engine) setPhase(rec *operation.Record, phase string) error {
	rec.Phase = phase
	if rec.Result == nil {
		rec.Result = map[string]any{}
	}
	return e.ops().Save(*rec)
}

func (e *Engine) stop(phase string) bool {
	return e.StopAfter != "" && e.StopAfter == phase
}

func (e *Engine) compensate(ctx context.Context, rec operation.Record, liveMain, backup string, cause error) error {
	_ = e.setPhase(&rec, PhaseCompensating)
	if err := e.restoreFiles(liveMain, backup); err != nil {
		rec, _ = e.ops().Finish(rec, operation.StatusRecoveryRequired, PhaseCompensating, map[string]any{
			"rolled_back":  false,
			"config_state": StateUncertain,
			"cause":        cause.Error(),
		}, &operation.ErrorBody{Code: CodeRecovery, Message: err.Error()})
		return attachOp(wrap(CodeRecovery, "restore failed, manual recovery required: "+err.Error(), err), rec)
	}
	if err := cfgver.RestoreCertBackup(filepath.Join(backup, "certs"), e.certDir()); err != nil {
		rec, _ = e.ops().Finish(rec, operation.StatusRecoveryRequired, PhaseCompensating, map[string]any{
			"rolled_back":  false,
			"config_state": StateUncertain,
			"cause":        cause.Error(),
		}, &operation.ErrorBody{Code: CodeRecovery, Message: err.Error()})
		return attachOp(wrap(CodeRecovery, "restore certificates failed, manual recovery required: "+err.Error(), err), rec)
	}
	_ = e.setPhase(&rec, PhaseRestored)
	reloadErr := e.reloader().Reload(ctx)
	healthErr := e.health().Check(ctx)
	if reloadErr != nil || healthErr != nil {
		msg := "restored files but reload/health failed"
		if reloadErr != nil {
			msg = reloadErr.Error()
		} else {
			msg = healthErr.Error()
		}
		rec, _ = e.ops().Finish(rec, operation.StatusRecoveryRequired, PhaseRestored, map[string]any{
			"rolled_back":  false,
			"config_state": StateUncertain,
			"cause":        cause.Error(),
		}, &operation.ErrorBody{Code: CodeRecovery, Message: msg})
		return attachOp(wrap(CodeRecovery, msg, cause), rec)
	}
	_ = e.setPhase(&rec, PhaseRestoreVerified)
	rec, _ = e.ops().Finish(rec, operation.StatusFailed, PhaseRestoreVerified, map[string]any{
		"rolled_back":  true,
		"config_state": StateMatchesPrevious,
		"cause":        cause.Error(),
	}, &operation.ErrorBody{Code: CodeFailed, Message: cause.Error()})
	return attachOp(wrap(CodeFailed, cause.Error(), cause), rec)
}

func (e *Engine) fail(rec operation.Record, state string, cause error) error {
	rec, _ = e.ops().Finish(rec, operation.StatusFailed, rec.Phase, map[string]any{
		"rolled_back":  false,
		"config_state": state,
	}, &operation.ErrorBody{Code: CodeFailed, Message: cause.Error()})
	return attachOp(wrap(CodeFailed, cause.Error(), cause), rec)
}

func (e *Engine) unknown(rec operation.Record, cause error) error {
	rec, _ = e.ops().Finish(rec, operation.StatusUnknown, rec.Phase, map[string]any{
		"config_state": StateUncertain,
	}, &operation.ErrorBody{Code: CodeUnknown, Message: cause.Error()})
	return attachOp(wrap(CodeUnknown, cause.Error(), cause), rec)
}

func resultState(rec operation.Record) string {
	if rec.Result == nil {
		return ""
	}
	s, _ := rec.Result["config_state"].(string)
	return s
}

// Recover 对账失联的 running 记录：已切换文件的标 recovery_required，否则 unknown。不盲目重做。
func (e *Engine) Recover() error {
	list, err := e.ops().List(operation.ListFilter{Status: operation.StatusRunning, Limit: operation.MaxLimit})
	if err != nil {
		return err
	}
	for _, rec := range list {
		if e.State.IsAlive(rec.OwnerPID, rec.OwnerStartKey) {
			continue
		}
		status := operation.StatusUnknown
		cfg := StateUnchanged
		switch rec.Phase {
		case PhaseFilesSwitched, PhaseReloaded, PhaseVerified, PhaseCompensating, PhaseRestored:
			status = operation.StatusRecoveryRequired
			cfg = StateUncertain
		}
		if rec.Result == nil {
			rec.Result = map[string]any{}
		}
		rec.Result["last_phase"] = rec.Phase
		rec.Result["config_state"] = cfg
		_, err := e.ops().Finish(rec, status, rec.Phase, rec.Result, &operation.ErrorBody{
			Code:    statusCode(status),
			Message: "owner process gone",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func statusCode(status string) string {
	if status == operation.StatusRecoveryRequired {
		return CodeRecovery
	}
	return CodeUnknown
}
