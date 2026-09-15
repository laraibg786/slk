package main

import (
	"sync"

	"github.com/gammons/slk/internal/config"
)

// workspacesMu guards cfg.Workspaces once workspaces start connecting:
// saveTheme and saveSidebarWidth (wired in run) mutate it in place from
// the Update goroutine, while every connect goroutine takes an
// independent snapshot of it once, at connect time, via
// snapshotWorkspaces -- see its doc comment for why a value copy of
// config.Config on its own isn't enough.
var workspacesMu sync.RWMutex

// snapshotWorkspaces returns cfg with its own independent copy of
// Workspaces. A plain `cfgCopy := cfg` does not do this: Config is a
// value type, but Workspaces is a map, and copying a struct copies the
// map header, not its contents -- both copies still point at the same
// underlying map. Call this once per workspace connect, before handing
// cfg to anything that will read it (via WorkspaceByTeamID,
// MatchSectionAndOrder, SectionOrder) from a goroutine other than the
// one running saveTheme/saveSidebarWidth, so a later theme or
// sidebar-width save never races a concurrent read of the same map.
func snapshotWorkspaces(cfg config.Config) config.Config {
	workspacesMu.RLock()
	defer workspacesMu.RUnlock()
	clone := make(map[string]config.Workspace, len(cfg.Workspaces))
	for k, v := range cfg.Workspaces {
		clone[k] = v
	}
	cfg.Workspaces = clone
	return cfg
}
