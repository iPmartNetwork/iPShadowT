package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/iPmart/iPShadowT/internal/config"
	"github.com/iPmart/iPShadowT/internal/logger"
)

func TestFailoverCreation(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Transport: "reality", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Transport: "wsmux", Priority: 2},
		{Name: "tertiary", RemoteAddr: "3.3.3.3:443", Transport: "tcpmux", Priority: 3},
	}

	fo := NewFailover(paths, DefaultFailoverConfig(), log, events)
	if fo == nil {
		t.Fatal("NewFailover returned nil")
	}

	// Active path should be the highest priority (lowest number)
	active := fo.ActivePath()
	if active.Name != "primary" {
		t.Errorf("expected active path 'primary', got %q", active.Name)
	}
}

func TestFailoverReportFailure(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Transport: "reality", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Transport: "wsmux", Priority: 2},
	}

	cfg := DefaultFailoverConfig()
	cfg.FailThreshold = 2

	fo := NewFailover(paths, cfg, log, events)

	// First failure - should not trigger failover
	triggered := fo.ReportFailure(fmt.Errorf("connection timeout"))
	if triggered {
		t.Error("failover should not trigger on first failure")
	}

	// Second failure - should trigger failover
	triggered = fo.ReportFailure(fmt.Errorf("connection timeout"))
	if !triggered {
		t.Error("failover should trigger after reaching threshold")
	}

	// Active path should now be secondary
	active := fo.ActivePath()
	if active.Name != "secondary" {
		t.Errorf("expected active path 'secondary' after failover, got %q", active.Name)
	}
}

func TestFailoverReportSuccess(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Priority: 2},
	}

	cfg := DefaultFailoverConfig()
	cfg.FailThreshold = 3

	fo := NewFailover(paths, cfg, log, events)

	// Report some failures
	fo.ReportFailure(fmt.Errorf("err"))
	fo.ReportFailure(fmt.Errorf("err"))

	// Report success - should reset counter
	fo.ReportSuccess()

	// Now failures should need full threshold again
	triggered := fo.ReportFailure(fmt.Errorf("err"))
	if triggered {
		t.Error("failover should not trigger after success reset")
	}
}

func TestFailoverForceSwitch(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Priority: 2},
	}

	fo := NewFailover(paths, DefaultFailoverConfig(), log, events)

	err := fo.ForceSwitch("secondary")
	if err != nil {
		t.Fatalf("ForceSwitch failed: %v", err)
	}

	active := fo.ActivePath()
	if active.Name != "secondary" {
		t.Errorf("expected 'secondary' after force switch, got %q", active.Name)
	}
}

func TestFailoverDisableEnable(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Priority: 2},
	}

	cfg := DefaultFailoverConfig()
	cfg.FailThreshold = 1

	fo := NewFailover(paths, cfg, log, events)

	// Disable secondary
	fo.DisablePath("secondary")

	// Trigger failover from primary - should fail (no healthy paths)
	triggered := fo.ReportFailure(fmt.Errorf("err"))
	if triggered {
		t.Error("failover should not succeed when all alternatives are disabled")
	}

	// Re-enable secondary
	fo.EnablePath("secondary")

	// Now failover should work
	triggered = fo.ReportFailure(fmt.Errorf("err"))
	if !triggered {
		t.Error("failover should succeed after re-enabling path")
	}
}

func TestFailoverAllPaths(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "a", RemoteAddr: "1.1.1.1:443", Priority: 1},
		{Name: "b", RemoteAddr: "2.2.2.2:443", Priority: 2},
		{Name: "c", RemoteAddr: "3.3.3.3:443", Priority: 3},
	}

	fo := NewFailover(paths, DefaultFailoverConfig(), log, events)

	allPaths := fo.AllPaths()
	if len(allPaths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(allPaths))
	}
}

func TestFailoverRecovery(t *testing.T) {
	log := logger.New("error")
	events := NewEventBus()

	paths := []config.PathConfig{
		{Name: "primary", RemoteAddr: "1.1.1.1:443", Priority: 1},
		{Name: "secondary", RemoteAddr: "2.2.2.2:443", Priority: 2},
	}

	cfg := DefaultFailoverConfig()
	cfg.FailThreshold = 1
	cfg.RecoverAfter = 100 * time.Millisecond
	cfg.HealthInterval = 50 * time.Millisecond

	fo := NewFailover(paths, cfg, log, events)
	fo.Start()
	defer fo.Stop()

	// Trigger failover
	fo.ReportFailure(fmt.Errorf("err"))

	active := fo.ActivePath()
	if active.Name != "secondary" {
		t.Fatalf("expected secondary after failover, got %q", active.Name)
	}

	// Wait for recovery
	time.Sleep(200 * time.Millisecond)

	// Primary should be marked as degraded (ready for recovery)
	allPaths := fo.AllPaths()
	for _, p := range allPaths {
		if p.Config.Name == "primary" {
			if p.State == PathUnhealthy {
				t.Error("primary should have recovered from unhealthy after RecoverAfter")
			}
		}
	}
}
