package modules

import (
	"fast-https/modules"
	"fast-https/modules/logging"
	"fast-https/modules/workchain"
	_ "fast-https/modules/workchain/example"
	"testing"
)

func TestDefaultLoggerRegisteredByImport(t *testing.T) {
	loggerModule, err := modules.GetModule[modules.Logger]("fast.plugin.DefaultLogger")
	if err != nil {
		t.Fatalf("default logger module should be available after package import: %v", err)
	}
	if loggerModule == nil {
		t.Fatal("default logger module should not be nil")
	}
}

func TestDuplicateModuleRegistrationRejected(t *testing.T) {
	ok := modules.RegisterModule(&logging.DefaultLogger{})
	if ok {
		t.Fatal("duplicate registration for default logger should be rejected")
	}
}

func TestWorkchainExampleModulesRegistered(t *testing.T) {
	m1, err := modules.GetModule[workchain.Handler]("fast.process.Gizmo")
	if err != nil || m1 == nil {
		t.Fatalf("fast.process.Gizmo should be registered and resolvable, err=%v", err)
	}

	m2, err := modules.GetModule[workchain.Handler]("fast.process.error")
	if err != nil || m2 == nil {
		t.Fatalf("fast.process.error should be registered and resolvable, err=%v", err)
	}

	m3, err := modules.GetModule[workchain.Handler]("fast.process.cache")
	if err != nil || m3 == nil {
		t.Fatalf("fast.process.cache should be registered and resolvable, err=%v", err)
	}
}
