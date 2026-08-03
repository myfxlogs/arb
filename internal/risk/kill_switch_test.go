package risk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKillSwitchInactiveByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kill_switch")
	ks := NewKillSwitch(path)
	if ks.IsActive() {
		t.Fatal("kill switch should be inactive by default")
	}
	if err := ks.Check(); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

func TestKillSwitchActivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kill_switch")
	ks := NewKillSwitch(path)
	if err := ks.Activate(); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !ks.IsActive() {
		t.Fatal("kill switch should be active after Activate")
	}
	if err := ks.Check(); err != ErrKillSwitch {
		t.Errorf("Check() = %v, want ErrKillSwitch", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("kill switch file not created: %v", err)
	}
}

func TestKillSwitchDeactivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kill_switch")
	ks := NewKillSwitch(path)
	_ = ks.Activate()
	if err := ks.Deactivate(); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if ks.IsActive() {
		t.Fatal("kill switch should be inactive after Deactivate")
	}
}

func TestKillSwitchExternalFileCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kill_switch")
	ks := NewKillSwitch(path)
	// Simulate external process creating the file
	f, _ := os.Create(path)
	f.Close()
	if !ks.IsActive() {
		t.Fatal("kill switch should detect external file creation")
	}
}
