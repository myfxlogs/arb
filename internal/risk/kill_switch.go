package risk

import (
	"errors"
	"os"
	"sync/atomic"
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker open")

// ErrKillSwitch is returned when the kill switch is active.
var ErrKillSwitch = errors.New("kill switch active")

// KillSwitch monitors a file on disk. When the file exists, all trading stops.
type KillSwitch struct {
	path    string
	active  atomic.Bool
	checking atomic.Bool
}

// NewKillSwitch creates a KillSwitch monitoring the given file path.
func NewKillSwitch(path string) *KillSwitch {
	return &KillSwitch{path: path}
}

// IsActive returns true if the kill switch file exists.
func (k *KillSwitch) IsActive() bool {
	if k.active.Load() {
		return true
	}
	_, err := os.Stat(k.path)
	if err == nil {
		k.active.Store(true)
		return true
	}
	return false
}

// Activate creates the kill switch file.
func (k *KillSwitch) Activate() error {
	k.active.Store(true)
	f, err := os.Create(k.path)
	if err != nil {
		return err
	}
	return f.Close()
}

// Deactivate removes the kill switch file.
func (k *KillSwitch) Deactivate() error {
	k.active.Store(false)
	return os.Remove(k.path)
}

// Check returns ErrKillSwitch if the kill switch is active, nil otherwise.
func (k *KillSwitch) Check() error {
	if k.IsActive() {
		return ErrKillSwitch
	}
	return nil
}
