package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gammons/slk/internal/config"
)

// TestSnapshotWorkspaces_IndependentOfSource proves the snapshot's map
// is a separate object from the source's: mutating the source after
// the call must never be visible through the snapshot. A value copy
// of config.Config alone does not give this -- see snapshotWorkspaces's
// doc comment -- so this pins the one property the fix actually
// depends on.
func TestSnapshotWorkspaces_IndependentOfSource(t *testing.T) {
	cfg := config.Config{Workspaces: map[string]config.Workspace{
		"acme": {TeamID: "T1", Theme: "dark"},
	}}
	snap := snapshotWorkspaces(cfg)

	workspacesMu.Lock()
	cfg.Workspaces["acme"] = config.Workspace{TeamID: "T1", Theme: "light"}
	workspacesMu.Unlock()

	if got := snap.Workspaces["acme"].Theme; got != "dark" {
		t.Fatalf("snapshot observed the source's later mutation: Theme = %q, want %q", got, "dark")
	}
}

// TestSnapshotWorkspaces_ConcurrentWithWriter is a race-detector test.
// It reproduces the pattern boot produces -- the Update goroutine
// mutating cfg.Workspaces in place (saveTheme/saveSidebarWidth) while
// every connect goroutine snapshots it once at connect time, for its
// own rtmEventHandler and initial channel-list build -- and fails
// under -race if snapshotWorkspaces or its caller ever touch the map
// without workspacesMu held. In production the unguarded version is a
// runtime fatal ("concurrent map read and map write"), not merely a
// report from the detector.
func TestSnapshotWorkspaces_ConcurrentWithWriter(t *testing.T) {
	cfg := config.Config{Workspaces: map[string]config.Workspace{}}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("T%d", i)
		cfg.Workspaces[id] = config.Workspace{TeamID: id}
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Simulated Update-goroutine writer: repeatedly mutates one entry,
	// the same read-modify-write shape as saveTheme/saveSidebarWidth.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			workspacesMu.Lock()
			ws := cfg.Workspaces["T0"]
			ws.Theme = "toggled"
			cfg.Workspaces["T0"] = ws
			workspacesMu.Unlock()
		}
	}()

	// Simulated connect goroutines: snapshot cfg and read the result,
	// the same shape as buildChannelItem/WorkspaceByTeamID afterward.
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("T%d", i)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				snap := snapshotWorkspaces(cfg)
				if _, ok := snap.Workspaces[id]; !ok {
					t.Errorf("snapshot missing %s", id)
				}
			}
		}(id)
	}

	time.Sleep(20 * time.Millisecond)
	close(stop)
	wg.Wait()
}
