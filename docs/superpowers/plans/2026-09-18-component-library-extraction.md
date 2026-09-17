# Component Library Extraction — tracking document

**Goal.** Turn `internal/ui/<widget>/` into a set of **fully independent,
domain-agnostic Bubble Tea components** that know nothing about Slack, nothing
about slk, and nothing about each other — such that they can eventually move to
`pkg/` and be imported by someone building a TUI over WhatsApp, Teams, Matrix or
IRC.

**Constraints this plan is written against.** Ship continuously; no feature
freeze; every PR short, single-purpose and reviewable in isolation; the tree
green at every commit.

**Measured at** `053d2a2` (`upstream/main`, 2026-09-17). Local `main` was 12
commits behind and has been fast-forwarded to it. Tree is green:
`CGO_ENABLED=0 go test ./internal/ui/...` passes. (Plain `go test` fails to
cgo-compile `golang.design/x/clipboard` on a box without `X11/Xlib.h` — an
environment gap, not a code problem.)

**Reference.** The target conventions are those in the *Bubble Tea Component
Architecture Guide* (`https://claude.ai/artifact/YB3XoaSfmu3CtawcEAkn66`),
§§2–8 and the §10 anti-pattern checklist. Where this plan deviates from the
guide, it says so and why.

**Relationship to `2026-09-06-architecture-refactor.md`.** That document
attacks **size and duplication**; this one attacks **coupling and portability**.
They overlap in exactly two places, flagged inline: its Phase 3 (`messages`/
`thread` fork) and Phase 4 (modal chrome). Nothing here contradicts it, and
Stage 6 below *is* its Phase 3 with a different destination.

---

## 1. Baseline

| Metric | Value at `053d2a2` |
|---|---|
| Component packages under `internal/ui/` | 30 (13 modals) |
| `internal/ui` root (the app layer), non-test | 13,561 lines |
| `App` → component call sites | **1,312** across 365 distinct methods |
| Exported methods on component `Model`s | 460 |
| `Set*` methods on component `Model`s | 106 |
| Components implementing `tea.Model` | **0** |
| Components with any `Update(tea.Msg)` | **1** (`compose`) |
| Components with `Init()` | **0** |
| Exported *fields* on component `Model`s | **0** |
| `styles.*` global reads across components | **612**, in 23 of 30 packages |
| `tea.Msg` types: app layer vs components | 43 vs 19 |
| `HandleKey` impls / distinct return types | 11 / **9** |

**The good news first, because it sets the cost of everything below.**

- **No component exposes a mutable field**, and **no component holds a service
  port.** Every one of `core`'s 14 service interfaces (`ChannelService`,
  `ThreadService`, …) is used *only* by the app layer. The I/O boundary is
  already clean and already enforced by `internal/ui/boundary_test.go`.
- **Component→component traffic is 11 lines** out of 13,561. `App` genuinely is
  a mediator today.
- **Each component touches only 1–7 `core` symbols**, and they are all *data
  types*, never ports:

  | Component | `core` symbols used |
  |---|---|
  | `messages` | `MessageItem`, `ReactionItem`, `Attachment`, `ThumbSpec` |
  | `sidebar` | `ReadState` (7 refs) |
  | `threadsview` | `ThreadSummary` (7 refs) |
  | `presencemenu` | `PresenceAction` + 5 constants |
  | `channelfinder` | `ChannelFinderItem` |
  | `compose` | `PendingAttachment` |
  | `reactionpicker` | `EmojiEntry` |
  | `styles` | `Theme` |
  | `themeswitcher` | `ThemeScope` + 2 constants |
  | `imgrender`, `blockkit` | `ImageFetcher` (already an injected interface) |

So the distance to a portable component library is far shorter than the 1,312
call sites suggest. The blockers are four, and they are not evenly sized.

---

## 2. What actually blocks extraction

### D1 — `internal/ui/styles` is a package-global mutable theme singleton
**Severity: highest. This is the single largest obstacle, and v1 of this
analysis missed it entirely.**

```go
// internal/ui/styles/styles.go:12
var (
    Primary     color.Color = lipgloss.Color("#4A9EFF")
    Background  color.Color = lipgloss.Color("#1A1A2E")
    SidebarText color.Color = lipgloss.Color("#E0E0E0")
    ... 20+ exported vars ...
)
var version int64                                   // :269
func Version() int64 { return version }             // :272
func Apply(themeName string, overrides core.Theme)  // :276 — mutates all of it
```

**612 reads across 23 of 30 component packages** — `messages` 93, `sidebar` 71,
`thread` 63, `statusbar` 31, down to 4 apiece for `workspace`, `emojipicker`,
`channelpicker`. Components reach into process-global mutable state to find out
what color to draw in.

This is the guide's §8 anti-pattern ("no package-level mutable state or
singletons used as a side channel") and the direct inverse of §6/§7 (each
component owns a local `Styles`/`KeyMap` with a `DefaultStyles()`/
`DefaultKeyMap()` constructor, as `list.Model` and `table.Model` do upstream).

No component can be extracted to `pkg/` while it reads a global, and no
component can be instantiated twice with different looks — which a split view
already wants.

The complication to respect: `Apply()` + `Version()` is a **working runtime
theme-switch and cache-invalidation protocol**. `styles.Version()` feeds the
per-pane render memo. Replacing the global cannot mean losing that.

### D2 — Shared `core` data types
**Severity: low. Much cheaper than it looks.**

Per the table in §1: 1–7 symbols each, all plain data. And the types are
*already* mostly domain-neutral chat concepts. `core.MessageItem`:

```go
type MessageItem struct {
    TS, ThreadTS         string   // Slack naming for "ID" / "parent ID"
    UserName, UserID     string   // neutral
    Text, Timestamp, DateStr string // neutral
    ReplyCount           int      // neutral
    Reactions            []ReactionItem  // neutral
    Attachments          []Attachment    // neutral
    IsEdited             bool     // neutral
    Subtype              string   // "mirrors Slack's `subtype` field"
    Blocks               []blocks.Block            // <-- Slack Block Kit
    LegacyAttachments    []blocks.LegacyAttachment // <-- Slack Block Kit
}
```

Everything is portable except **naming** (`TS` → `ID`, `ThreadTS` → `ParentID`),
`Subtype` (one consumer: rendering a `thread_broadcast` label — becomes a
neutral label/badge), and the two Block Kit fields, which are D3.
`ReactionItem`, `ThumbSpec`, `ReadState`, `ThreadSummary`, `EmojiEntry`,
`PendingAttachment` are already neutral; `Attachment.FileID` is documented as a
Slack file ID but is used only as an opaque cache key.

Go **type aliases** make this stage nearly free — see Stage 3.

### D3 — Block Kit is welded into the message model
**Severity: medium, but the seam is unusually clean.**

`blockkit` is the one component that is irreducibly Slack: it imports `slack-go`
as the data it parses, which is correct and should stay. The problem is that
`messages` and `thread` embed its output type in their message model.

But the consumption surface is tiny. `blockkit`'s whole exported API is 7
functions (`Parse`, `ParseAttachments`, `Render`, `RenderLegacy`, `RendersBody`,
`ResolveAttachmentColor`, `RichTextToMrkdwn`), and `messages` + `thread` call it
at **6 places**:

```
messages/model.go:531,548,552  RichTextToMrkdwn fallback + RendersBody
messages/model.go:1911         blockkitContext(...)
messages/model.go:2241,2267    Render / RenderLegacy
thread/model.go:1861           blockkitContext(...)   (mirror)
thread/model.go:1941,1947      Render / RenderLegacy  (mirror)
```

Two injected interfaces cover all six: a **body renderer** (`Render`,
`RenderLegacy`, `RendersBody`) and a **plain-text fallback**
(`RichTextToMrkdwn`, used to reconstruct body text when the `text` field is
empty). `blockkit` then lives in slk as the Slack *adapter* for those
interfaces, and a WhatsApp client injects its own — or none, and gets plain
text.

### D4 — App infrastructure leaks into components

- **`debuglog`** is called from 4 components: `messages` (17), `imgrender` (18),
  `thread` (10), `sidebar` (4). A library cannot hard-depend on slk's logger.
- **`SendMsg func(any)` / `func(tea.Msg)`** in `imgrender.ImageContext:43`,
  `blockkit/types.go:102`, `emoji/place.go:33`, captured and fired from
  goroutines at `messages/model.go:1902` and `thread/model.go:1866`. This is the
  program's `p.Send` smuggled through a struct field — the guide's §8 "never
  store a `*tea.Program` on a Model", one level of indirection removed. It is
  also the *cause* of two things the other plan records separately: the double
  `SetImageContext`/`SetEmojiContext` calls from `main.go` (its F4
  "bootstrap-ordering smell") and the closure-capture races (its F2).

### D5 — Carried forward from v1, unchanged by the new goal

- **No component participates in the Bubble Tea contract.** No `Init`; `Update`
  only on `compose`; `View(height, width int)` rather than `View() string`, so
  **no component owns its own size**. Guide §2, §5.5.
- **`View()` mutates.** Seven pushes happen during render —
  `view_messages.go:71,91,122,123`, `view_sidebar.go:42`, `view_thread.go:41,44`
  — plus `App.View()` writing `a.threadVisible` at `app.go:3096`. Guide §10.
- **Keys arrive as strings** via `normalizeFinderKey`, losing modifiers, and
  `HandleKey` has 9 distinct return types, which is precisely why no modal
  interface is declarable today. Keybindings live in the *app's* `KeyMap`, not
  the component's. Guide §7.
- **`thread` depends on `messages` as a type**, not a contract: 99 refs to
  `messages.MessageItem`, 23 to `messages.Model`; 377 verbatim shared lines; a
  lockstep test pinning one static 80×20 state with 15 documented divergences.
- **Multi-instance children are untagged.** Two `compose` instances and N
  `winModels` message panes, routed by mode/focus rather than by tagged
  messages. Guide §5.4. (`ImageReadyMsg{Channel, TS, Key}` is accidental
  self-tagging and works; the `compose`/`threadCompose` pair has nothing.)
- **`App` knows component data shapes.** `App.SetUserNames` (`app.go:2790`)
  builds `mentionpicker.User` values itself and fans six directory maps out to
  every pane, with `winmodels.go:newWindowModel` hand-replaying all of them onto
  late-created windows.

---

### D6 — The UI layer owns quota policy that belongs to `core`

**Severity: high. This is a layering inversion, not just a coupling one.**

The principle: **the UI must not know what a rate limit is.** Bandwidth,
quotas, retry, freshness and request deduplication are `core`'s job. A
component's business is "I need this conversation's recent messages"; whether
that costs an API call is not its concern, and a portable component library
cannot encode Slack's rate tiers.

Today `App` owns all of it. Roughly 15 fields are pure transport policy sitting
in the presentation layer:

| `App` field | What it really is |
|---|---|
| `markFlushDebounce`, `markFlushScheduled` | rate limiter for a Tier 3 endpoint |
| `selfMarks`, `selfThreadMarks` | echo dedup for self-issued writes |
| `fetchingOlder map[string]bool` | hand-rolled per-channel single-flight |
| `channelSearchDebounce`, `pendingChannelSearchGen` | request coalescing |
| `pendingThreadFetchGen` | request coalescing |
| `threadsDirtyDebounce`, `threadsListFetchScheduled` | request coalescing |
| `emojiInvalidatePending` | burst coalescing |
| `newMessageInFlightID` | in-flight generation guard |

`ChannelService.SearchRemote`'s own doc comment admits the leak: *"Callers run
it from a Cmd, debounced — see `App.scheduleChannelSearch`."*

Note this **supersedes** the other plan's Phase 5 treatment, which counts "8
debounce/generation counters" as residue to be extracted into a `debounce`
struct *still on `App`*. Under this principle they should not be on `App` at
all.

#### The three layers

```
component  ->  a narrow interface it defines itself; states INTENT only
core       ->  cache, single-flight, rate limiting, retry, freshness
App        ->  cross-component reconciliation only
```

Components express a freshness *need*; `core` owns the mechanism:

```go
// component-side: naive, portable, no policy, no knowledge of quotas
type Loader interface {
    Load(convID string, maxAge time.Duration) tea.Cmd
}
```

`core` then does cache -> single-flight -> network internally.
`golang.org/x/sync/singleflight` is already in the module graph (x/sync
v0.14.0), so the dedup is an import rather than more hand-rolling.

**Do not make the read ports implicitly cached.** If `Fetch` silently serves
cache, callers lose the ability to force a refresh — which reconnect-sync and
the cache-verify tier both need. The existing `ReadCache` / `SyncedAt` /
`Fetch` triad gets the explicitness right; the fix is to move the *tiering
decision* into `core` behind a `maxAge` argument, not to hide the cache.

#### Effect-ownership test

For each effect, ask **where its result lands** and **whose policy governs it**:

| Result lands | Policy | Owner | Examples |
|---|---|---|---|
| one component | local | **component** | finder search debounce, thread-fetch gen, reactions, link open, file download, emoji invalidate |
| one component | shared transport | **component calls, `core` throttles** | history load, older-history backfill, thumbnails |
| many components | shared transport | **`core` throttles, `App` fans out** | mark-read, incoming message, workspace switch |
| many components | none | **`App`** | focus/blur, layout, mode arbitration, window tree |

#### Two subtleties that a naive move would break

1. **Focus gating is not rate limiting.** `pendingChannelMark` /
   `pendingThreadMark` combine two different concerns: *should this mark happen
   at all* (is the user looking? `terminalFocused`, `autoMarkArmed`, `inTmux` —
   genuinely UI knowledge, and the tmux focus-events case is delicate) and *how
   often may marks go out* (`core`'s). Split the field: staging stays in the UI,
   flushing moves to `core`. `newMessageCancelled` is likewise UI intent (the
   user pressed Esc) and stays.

2. **This trades free concurrency-safety for explicit locking.** Those 15 fields
   are goroutine-safe today *by construction* — only the update loop touches
   them. Moving them into `core` means `Cmd` goroutines touch them, so they need
   real synchronization. That is the right trade (one mutexed owner beats 15
   fields coordinated by convention), but the other plan's F2 records three
   existing data races in exactly this closure-capture style, so the locking
   must be designed rather than inherited.

## 3. Target layout

```
pkg/chatui/                  the component library — no slk, no Slack, no siblings
  message/                   message list pane   (from ui/messages, neutralized)
  thread/                    thread pane         (or one pane w/ a mode)
  conversationlist/          channel list        (from ui/sidebar)
  compose/
  statusbar/  rail/  finder/  picker/  confirm/  help/  ...
  internal/                  shared-by-necessity substrate (see §3.1)
    scrollbar/  selection/  wintree/  overlay/  term/

internal/                    slk: the Slack application
  slackui/                   ADAPTERS: Slack -> chatui contracts
    blockkit/                Slack Block Kit -> chatui BodyRenderer
    mrkdwn/  peerstatus/  usergroups/
  ui/                        the app: root model, routing, layout, reducers
  core/                      service ports + slk domain types
```

### 3.1 The one deliberate deviation from the guide

Guide §3 says components should share **no** low-level package. This plan keeps
a small `pkg/chatui/internal/` substrate (`scrollbar`, `selection`, `wintree`,
terminal-width helpers) and, per the guide's own carve-out, a neutral data
package for "data that isn't UI at all".

Rationale: these are already dependency-free, they are *substrate* not
*widgets*, and Go's `internal/` keeps them unimportable from outside the
library, so they never become part of its public contract. This is AGENTS.md's
"extract the substrate, not the widget" applied at the library boundary. The
guide's rule is aimed at components reaching sideways for *behavior* (keymaps,
themes) — which this plan does not do: `KeyMap` and `Styles` stay per-component
per §6/§7.

### 3.2 Portability triage

| Already portable (zero slk deps) | Lines | Action |
|---|---|---|
| `selection`, `scrollbar`, `wintree` | 594 | move as-is — Stage 1 |
| `internal/image` (sixel/kitty/halfblock) | 3,175 | generic terminal-image lib; move later |
| `internal/text` (case/accent fold) | 90 | move as-is |
| `internal/emoji` | 5,232 | generic for any chat TUI; move later |

| Needs neutralizing | Blocker |
|---|---|
| `messages`, `thread` | D1, D2, D3, D5 — the heavy ones |
| `sidebar`, `threadsview`, `channelfinder`, `compose`, and 12 modals | D1, D2, D5 |
| `styles` | **dissolves** — becomes per-component `Styles` + app-side palette |

| Stays slk-only (correctly) | Why |
|---|---|
| `blockkit` | Slack Block Kit is Slack's; becomes an adapter |
| `slack/mrkdwn`, `slackurl`, `ids`, `usergroups` | Slack wire formats |
| `peerstatus` | DND + huddles are Slack concepts; adapter feeding a neutral presence type |
| `internal/core` | slk's ports; the app layer's own |

---

## 4. Stepwise plan

Ordering principles, in priority order:

1. **Guardrails before motion.** The enforcement test lands first with
   skip-lists at today's state, so it passes on day one and every later PR
   *shrinks a list*. That is what makes this safe to do incrementally while
   other people ship features.
2. **Cheap and independent before expensive and entangled.** Stages 1–4 are
   mechanical and unblock each other; Stage 6 is the risky one and goes last.
3. **Cold components before hot ones.** Feature PRs concentrate in `messages`,
   `thread` and `sidebar`. Doing `workspace` (4 style reads) before `messages`
   (93) validates each pattern on a file nobody else is editing, so the pattern
   is settled before it reaches the contended files. **This is the main
   mechanism for not freezing features.**
4. **Type aliases over renames.** Go aliases let a type change owner without
   touching a single call site, which is what keeps Stage 3 to a handful of
   lines per PR.

Sizes below are a shape, not a promise. "Goldens green" means the 8 App-level
goldens byte-identical unless stated.

### Stage 0 — Guardrails (2 PRs, no production code)

| PR | Content | Size |
|---|---|---|
| 0.1 | `internal/ui/component_boundary_test.go`, modelled on the existing `boundary_test.go` AST walk. Four checks, each with a skip-list populated to today's state so it passes immediately: (a) no component imports a sibling component; (b) no component imports app infra (`debuglog`, `core`); (c) no component reads `styles.*`; (d) no `func(any)`/`func(tea.Msg)` struct fields. | ~250 lines, test-only |
| 0.2 | Per-component golden harness. Today's 8 goldens are all App-level, so a component-only change can only be verified through the whole app. A `golden_test.go` per component (fixed size, pinned theme/clock, reusing `newGoldenApp`'s nondeterminism pinning) makes every later PR self-verifying. | ~300 lines, test-only |
| 0.3 | Per-component `bench_test.go` covering `Update` and `View` with `b.ReportAllocs()`, and a recorded pre-move baseline. These run per keystroke and per frame respectively, and a component extraction changes what they allocate — measure before moving, not after a regression report. See `internal/bubbles/confirmprompt/bench_test.go` for the shape. | ~60 lines per component |

**Why first:** 0.2 is what lets Stages 2–5 be one-component-per-PR with local
proof. Without it every PR's blast radius is the whole app.

### Stage 1 — Free wins: move what is already portable (3 PRs)

| PR | Content |
|---|---|
| 1.1 | Create `pkg/chatui/internal/`; move `selection` (70 lines) + `scrollbar` (111). Pure move + import rewrite. Establishes the target layout with zero risk. |
| 1.2 | Move `wintree` (413). Same. |
| 1.3 | Move `internal/text` → `pkg/chatui/internal/fold` (90). Same. |

These three prove the layout, the enforcement test, and the review rhythm on
594 lines that cannot break rendering.

### Stage 2 — Evict app infrastructure (4 PRs, small)

| PR | Content |
|---|---|
| 2.1 | Replace `SendMsg func(any)` with a declared `Notifier interface { Send(tea.Msg) }`, wired **once** after `tea.NewProgram`. Fixes D4 and removes the reason `SetImageContext`/`SetEmojiContext` are each called twice from `main.go`. Coordinate with the other plan's Phase 2. |
| 2.2 | Optional `Logger` interface (or drop the calls) in `imgrender` (18) + `sidebar` (4). |
| 2.3 | Same for `messages` (17). |
| 2.4 | Same for `thread` (10). |

### Stage 3 — Neutral data types via aliases (≈9 PRs, tiny)

The mechanism, per component, so call sites never churn:

```go
// step 1 — component declares its own name for the type it needs
// pkg/chatui/conversationlist/types.go
type ReadState = core.ReadState        // alias: zero call-site changes

// step 2 — invert ownership; core keeps an alias for the app layer
// pkg/chatui/conversationlist/types.go
type ReadState struct { LastReadID string; HasUnread bool; MentionCount int }
// internal/core/types.go
type ReadState = conversationlist.ReadState   // until the app migrates
```

Order, cheapest first: `sidebar`/`ReadState` → `threadsview`/`ThreadSummary` →
`channelfinder` → `compose` → `reactionpicker` → `presencemenu` →
`themeswitcher` → `styles`/`Theme` → `messages` (4 types, last). Slack-flavored
field names (`TS` → `ID`, `ThreadTS` → `ParentID`) are renamed in the step-2 PR
for that type, not in a separate sweep.

### Stage 4 — The Block Kit seam (3 PRs)

| PR | Content |
|---|---|
| 4.1 | Declare in `messages`: `BodyRenderer` (`Render`, `RenderLegacy`, `RendersBody`) and `TextFallback` (`RichTextToMrkdwn`). Route the 6 call sites through them. `blockkit` becomes the injected Slack implementation. `Blocks` field unchanged — this PR is behavior-neutral and reviewable on its own. |
| 4.2 | Replace `Blocks []blocks.Block` + `LegacyAttachments` with an opaque payload the renderer interprets. `messages` loses `core/blocks` and `slack-go` from its graph. |
| 4.3 | Same for `thread` — **or** skip if Stage 6 lands first, since the fork collapse subsumes it. |

### Stage 5 — Dissolve the theme global (1 + ~23 PRs, each tiny) — the long pole

| PR | Content |
|---|---|
| 5.1 | **Mechanism only.** `styles.Snapshot() Styles` returns an immutable value; `App` owns theme application and re-pushes on change, bumping each component's version so the render memo still invalidates. Globals stay in place, so nothing breaks and nothing else changes yet. |
| 5.2 … 5.24 | **One component per PR:** add a local `Styles` struct + `DefaultStyles()`, replace that component's `styles.*` reads with `m.Styles.*`, have `New()` default it and `App` push the snapshot on construct + theme change. Goldens green each time. |

Order by read count, coldest first — `workspace` (4), `emojipicker` (4),
`channelpicker` (4), `confirmprompt` (5), `mentionpicker` (6), `reactionsview`
(6), `linkpicker` (8), `themeswitcher` (12), `reactionpicker` (12),
`workspacefinder` (13), `imgrender` (13), `presencemenu` (17), `blockkit` (17),
`help` (18), `newmessagepicker` (19), `channelfinder` (19), `threadsview` (20),
`searchresults` (21), `compose` (24), `statusbar` (31), then the contended three:
`thread` (63), `sidebar` (71), `messages` (93).

**Why this is 24 PRs and not 1:** 612 call sites in one diff is unreviewable and
would conflict with every in-flight feature branch. One component at a time is
~5–90 lines, mechanical, independently verifiable against its Stage-0.2 golden,
and rebases cleanly. Runtime theme switching keeps working throughout because
5.1 establishes the fan-out before any component stops reading the global.

Visual consistency afterwards is the app's job (guide §6): `internal/ui` holds
the palette and overrides each component's `Styles` after construction. The
palette lives in the app, never in the library.

### Stage 5.5 — `core` absorbs quota and freshness policy (5 PRs)

**Addresses D6. Hard prerequisite for Stage 6.2** — it must land before any
component issues its own service call, or the first migrated component is the
one that blows the mark budget.

| PR | Content |
|---|---|
| 5.5.1 | Single-flight in the read ports (`Fetch`, `FetchOlder`, `FetchAround`, thumbnails) via `x/sync/singleflight`. Retires `App.fetchingOlder`. Behavior-neutral: today's callers are already serialized by the update loop. |
| 5.5.2 | Fold the cache-first tiering into the read ports behind a `maxAge` argument; `ReadCache`/`SyncedAt` stay for callers that want the explicit form. Retires `cacheFreshThreshold` from the UI. |
| 5.5.3 | Move write rate limiting into `core`: the `conversations.mark` Tier 3 budget, `markFlushDebounce`/`markFlushScheduled`, and self-mark echo dedup (`selfMarks`, `selfThreadMarks`). **Split `pendingChannelMark`/`pendingThreadMark`** — focus-gated staging stays in the UI, flushing moves to `core`. Highest-risk PR in this stage; the focus-gating tests are the gate. |
| 5.5.4 | Move request coalescing for search and thread/threads-list fetches into `core` (`channelSearchDebounce`, `pendingChannelSearchGen`, `pendingThreadFetchGen`, `threadsDirtyDebounce`, `threadsListFetchScheduled`). |
| 5.5.5 | Synchronization audit of the above under `-race`, with the F2 races in scope. Do not skip — this stage is what makes `core` state concurrently reachable. |

### Stage 6 — The Bubble Tea contract (≈15 PRs)

| PR group | Content |
|---|---|
| 6.1 | `SetSize(w, h)` + `View() string` on the panes; relocate the 7 render-time pushes and `app.go:3096`'s write into the resize path. `View` becomes pure. Guide §5.5. |
| 6.2 (×11) | **Per modal, one PR:** local `KeyMap` + `DefaultKeyMap()`; `HandleKey(string)` → `Update(tea.Msg) (Model, tea.Cmd)`; the outcome becomes a `tea.Msg` declared in the component's own package (guide §5.3); retire that modal's use of `normalizeFinderKey`. Concrete return types, not `tea.Model` (guide §5.2). **Each PR also absorbs that component's own service calls and local debounce state** (D6, row 1 of the effect-ownership table) via a narrow component-defined interface injected with functional options — so expect ~150 lines per PR, not ~50. Requires Stage 5.5. |
| 6.3 | Tag multi-instance children: `compose`/`threadCompose` and the `winModels` panes. Guide §5.4. |
| 6.4 | Declare `Component`/`Focusable`/`Sizable`/`Overlay`; add `var _ Overlay = (*Model)(nil)` assertions. Promote the existing `boxedOverlay`/`clickableOverlay` (`reducer_modal_click.go:31,38`) from mouse-routing-only to the real contract. **This is the other plan's Phase 4** — same edit; do them together. |
| 6.5 | Convert the 11 cross-component call sites to routed messages, and fold `mode_channel_finder.go:38`'s `a.sidebar.SelectByID` into the `ChannelSelectedMsg` that `reducer_channels` already handles. |

### Stage 7 — `Directory`: replace push fan-out with pull (3 PRs)

A read-only interface the components take at construction, replacing the six
directory maps, their `App` retention fields, `newWindowModel`'s 10-call replay,
and `App`'s knowledge of `mentionpicker.User`:

```go
type Directory interface {
    UserName(id string) string
    ChannelName(id string) string
    CustomEmoji(name string) (string, bool)
    IsExternal(id string) bool
    Avatar(id string) string
    Presence(id string) Presence   // neutral; slk's peerstatus adapts DND/huddle
    Version() uint64               // one invalidation signal, not six pushes
}
```

`mentionpicker` builds its own entries from it. Under D6 this is no longer a
special case: `Directory` is simply one more narrow, component-defined port
injected by the same functional-options mechanism as `Loader` and `Marker`, and
its `Version()` is the same cache-invalidation signal. That makes this stage
smaller than originally scoped. This is also most of the other plan's Phase 5 —
**coordinate, don't duplicate.**

### Stage 8 — Collapse the `messages`/`thread` fork (highest risk, last)

The other plan's Phase 3, with a destination: one `pkg/chatui/message` pane
substrate that both consume as peers, and `thread` no longer importing
`messages` **at all** — thinning that edge is not enough. Resolve the real fork
first: `thread` uses `bubbles/viewport`, `messages` hand-rolls `yOffset`; pick
one. `lockstep_test.go`'s 15 documented divergences are the specification.
`TestLockstep_ReactionHitTestFrames` is a tripwire that fires on *convergence* —
**delete it, don't satisfy it.**

Deliberately last: it is the highest-risk change, it is where feature work
concentrates, and Stages 3–6 remove most of what makes the two files differ.

### Stage 9 — Physical move to `pkg/` (mechanical)

Only once the Stage-0.1 skip-lists are empty. At that point the move is an
import rewrite, and `pkg/chatui` is publishable.

---

## 5. Sequencing against live feature work

- **The skip-list is the contract.** A feature PR that adds a `styles.*` read to
  an unmigrated component passes; one that adds it to a migrated component
  fails. No coordination meeting required, and no branch has to be held.
- **Cold-first ordering** (§4 principle 3) keeps Stages 2–5 out of `messages`,
  `thread` and `sidebar` until the pattern is settled — so the files feature
  work touches are migrated last, when the diff is a known quantity.
- **Stages 1–5 are behavior-preserving by construction**, gated on
  byte-identical goldens. They are safe to merge during a release.
- **Stages 6 and 8 are not**, and should land early in a cycle, one component
  per PR, never two in the same release.
- **AGENTS.md updates ship with the PR that changes the thing**, not after.
  An unlisted helper gets re-implemented — that is the other plan's F7 cause #2,
  already diagnosed in this repo.

## 6. Enforcement

Each check lands in Stage 0.1 with a full skip-list and is *tightened by* the
stage that discharges it:

| Check | Discharged by |
|---|---|
| No component imports a sibling component | Stages 1, 4, 8 |
| No component imports `debuglog`/`core` | Stages 2, 3 |
| No component reads `styles.*` | Stage 5 |
| No `func(any)`/`func(tea.Msg)` struct field | Stage 2.1 |
| `Update` and `View` carry an allocation-reporting benchmark | Stage 0.3, then per component |
| `View` takes no geometry parameter | Stage 6.1 |
| Every modal satisfies `Overlay` (compile-time assertion) | Stage 6.4 |
| No component package imports `pkg/chatui/internal/...` from outside the library | Go enforces |

Plus the guide's §10 checklist as the review checklist for any PR touching
`pkg/chatui`.

## 7. Leave alone

- **`App` as mediator for *reconciliation*.** 11 lines of 13,561 do
  component→component work; do not dissolve that into an event bus. But note
  D6: `App` should stop being a mediator for *data and transport*. The measured
  split is that ~18 of the ~28 non-test reducer/mode files touch 0–1 components
  (pure couriering, which components absorb) while ~10 touch 2+ and are the
  highest-churn files in the repo (`reducer_workspace.go` 20 commits/9
  components, `reducer_io.go` 15/5, `mode_normal.go` 13/12). Those are
  irreducibly fan-out and stay. Expect this refactor to shrink `App`'s surface
  but **not** to move it off the top of the churn list.
- **`internal/core`'s service ports and `boundary_test.go`.** The outward
  boundary is the part of this architecture that is already right.
- **`Version()`.** It looks like leaked cache internals; it is a working render
  memo used by 6 panes. Keep it, and give `Directory` one.
- **`blockkit` importing `slack-go`.** Correct — it is the data it parses.
- **Unifying `Open(...)` (6 signatures) or `HandleKey`'s behavior (9 return
  types).** They diverge for real reasons. Extract the substrate, not the widget.

## 8. Open questions

1. **`viewport` or hand-rolled `yOffset`?** Blocks Stage 8 and sets the pane
   substrate's shape. Decide before Stage 6.1, since `SetSize` lands on it.
2. **How neutral should presence be?** `peerstatus` models DND and huddles as
   first-class. A neutral `Presence` with an extension escape hatch, or a
   generic component parameterised over a presence interface? Affects Stage 7.
3. **Does `emoji` (5,232 lines) belong in the library or beside it?** Shortcode
   maps are universal to chat TUIs; the custom-emoji *fetch* is not.
4. **One pane with a mode, or two panes?** Stage 8 could collapse `messages` and
   `thread` into one component with a "threaded" mode rather than two sharing a
   substrate. Cheaper, but risks the over-abstraction the other plan warns about.
5. **Module boundary.** `pkg/` in the same module, or a separate module from the
   start? Separate makes the dependency direction unforgeable but complicates
   local development.

## 9. Status

| Stage | Scope | PRs | Prereqs | Risk | Status |
|---|---|---|---|---|---|
| 0 | Guardrails | 3 | — | none | 0.3 shape landed (confirmprompt) |
| 1 | Move already-portable packages | 3 | 0.1 | none | not started |
| 2 | Evict app infra (`Notifier`, logger) | 4 | 0.1 | low | not started |
| 3 | Neutral data types (aliases) | ~9 | 0.1 | low | not started |
| 4 | Block Kit seam | 3 | 0.2 | medium | not started |
| 5 | Dissolve theme global | ~24 | 0.2 | low each, long | not started |
| 5.5 | `core` absorbs quota policy | 5 | 3 | medium-high | not started |
| 6 | Bubble Tea contract | ~15 | 0.2, 5, **5.5** | medium-high | not started |
| 7 | `Directory` pull model | 3 | 3, 5.5 | low | not started |
| 8 | Collapse `messages`/`thread` fork | ~6 | 5, 6 | **high** | not started |
| 9 | Physical move to `pkg/` | ~3 | all | none | not started |

Roughly **75 PRs**, the large majority of them small and mechanical. Stages 0–5
(45 PRs) are behavior-preserving and carry no feature-freeze cost. Stage 5.5 is
the first that changes real behavior (request timing), and Stage 6 is the first
that changes component contracts.
