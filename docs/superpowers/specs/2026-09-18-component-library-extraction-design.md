# Self-contained UI components in `internal/bubbles/`

Design for turning `internal/ui/<widget>/` into self-contained, domain-agnostic Bubble Tea components under `internal/bubbles/<widget>/`: each owning its own `Styles`, `KeyMap` and size, and taking the callables it needs as explicit parameters to `New` so it can issue its own commands instead of `App` doing that for it.

This RFC does **not** propose publishing a library. `internal/bubbles/` is the destination; whether any of it later moves to `pkg/` is a separate decision taken after the conventions have proven themselves in-tree.

## Problem

slk has 29 component packages and a 13,561-line app layer. The components look self-contained and are not. Measured at `4d4365b` from each package's non-test `GoFiles`:

**The theme is a process-global mutable singleton.** `internal/ui/styles` exports 20-odd `color.Color` vars plus `Apply()` that mutates them, and components read it **394 times across 23 of the 29 packages** — `messages` 72, `thread` 49, `statusbar` 29, `sidebar` 26. A component that reaches into a global for its colours cannot be instantiated twice with different looks, which a split view already wants, and cannot be reasoned about in isolation.

**No component participates in the Bubble Tea contract.** Zero implement `tea.Model`, zero have `Init()`, exactly one (`compose`) has `Update`. Every `View` takes geometry — `View(width int)`, `View(height, width int)`, `View(termWidth, termHeight int)` — so no component owns its own size. Construction is ad-hoc: six different `Open(...)` signatures, no common shape. Keys arrive as pre-normalized strings, losing modifiers, and `HandleKey` has nine distinct return types, which is why no modal interface is declarable.

**Components cannot act; they can only be driven.** No component holds a service port — every one of `core`'s 14 service interfaces is used only by `App`. That sounds like clean layering, and at the I/O boundary it is, but it means a widget cannot express *"the user asked me to mark this read"* as anything but a return value for `App` to interpret. The consequence is measurable in the reducers: of 30 non-test `reducer_*`/`mode_*` files, **12 touch exactly one widget** and are largely couriering between it and one service. (Four touch no widget at all; the remaining 14 fan out to two or more and are a different problem — see Non-goals.)

**The app layer owns transport policy.** Roughly 15 `App` fields are rate limiting, single-flight, request coalescing and echo dedup — `markFlushDebounce`, `fetchingOlder`, `channelSearchDebounce`, `pendingThreadFetchGen` and the rest. `ChannelService.SearchRemote`'s own doc comment admits it: *"Callers run it from a Cmd, debounced — see `App.scheduleChannelSearch`."*

**Slack types are embedded in the component data model.** `core.MessageItem` carries `Blocks []blocks.Block` and `LegacyAttachments`, so `messages` and `thread` transitively depend on `slack-go`. The rest is neutral chat vocabulary wearing Slack names — `TS`, `ThreadTS`.

**App infrastructure leaks in.** Four components call `debuglog` directly. Three carry a `SendMsg func(any)` field fired from goroutines — the program's `p.Send` smuggled through a struct, one indirection from storing a `*tea.Program` on a Model.

**Components import each other.** 14 of 29 import `messages`. Twelve do so only for leaf helpers (`BgANSI`, `FgANSI`, `ReapplyBgAfterResets`); only `reactionpicker` and `thread` touch the widget itself.

What is *not* broken bounds the cost: no component exposes a mutable field, the outward I/O boundary is already enforced by `boundary_test.go`, component→component *calls* are 11 lines out of 13,561, and each component touches only 1–7 `core` symbols, all plain data.

## Goals

- Components live in `internal/bubbles/<widget>/`, self-contained: no Slack types, no slk infrastructure, no sibling widgets.
- Uniform construction: `New(deps..., opts ...Option)` — required dependencies in the signature so the compiler enforces them, optional configuration in bubbles-style options.
- Widgets own their `Styles`, `KeyMap` and size.
- **A widget holds the callables it needs and issues its own `tea.Cmd`s.** The 12 single-widget reducer files collapse into the widgets they were couriering for.
- `core` owns cache, single-flight, rate limiting and freshness, so a widget states intent without knowing what a rate limit is.
- Ship continuously: no feature freeze, tree green at every commit.

## Non-goals

- **Publishing anything.** No `pkg/`, no versioned API, no external consumers. See Future possibilities.
- Dissolving `App` as a mediator for *reconciliation* (defined under Proposal). The 14 reducer files touching two or more widgets are irreducibly fan-out and stay — including the largest of them (`mode_normal.go` reaches 12 widgets, `reducer_workspace.go` and `reducer_modal_click.go` 9 apiece). This RFC shrinks `App`'s surface; it will not move it off the top of the churn list.
- Changing `internal/core`'s service ports or the outward I/O boundary.
- Making `blockkit` portable. Parsing Block Kit is Slack's job; it becomes an adapter.
- Unifying `HandleKey`'s nine return types by fiat. They converge as a consequence of adopting `Update`, not as a separate exercise.

## Proposal

### Target layout

```
internal
├── bubbles
│   ├── ansi
│   ├── channelfinder
│   ├── channelpicker
│   ├── compose
│   ├── confirmprompt
│   ├── conversationlist
│   ├── emojipicker
│   ├── fold
│   ├── help
│   ├── imgrender
│   ├── linkpicker
│   ├── mentionpicker
│   ├── message
│   ├── newmessagepicker
│   ├── overlay
│   ├── presencemenu
│   ├── reactionpicker
│   ├── reactionsview
│   ├── scrollbar
│   ├── searchresults
│   ├── selection
│   ├── statusbar
│   ├── themeswitcher
│   ├── thread
│   ├── threadsview
│   ├── wintree
│   ├── workspace
│   └── workspacefinder
├── core
│   └── blocks
├── slackui
│   ├── blockkit
│   ├── mrkdwn
│   └── peerstatus
└── ui
```

`bubbles/` holds both the widgets and the substrate they share — `ansi`, `fold`, `overlay`, `scrollbar`, `selection`, `wintree` — as flat siblings. There is no nested `internal/`: everything here is already unimportable from outside the module, and a second `internal/` would only add the narrower guarantee that `ui/` cannot reach the substrate either. That is not worth an awkward `internal/bubbles/internal/` path, and if the guarantee is ever wanted, the boundary test being added anyway can assert it in one line.

`slackui/` holds the adapters that satisfy widget-declared contracts with Slack implementations. `ui/` remains the app. `styles/` does not appear: it dissolves into per-widget `Styles` plus a palette owned by `ui/`. `sidebar` becomes `conversationlist` and `messages` becomes `message`, since neither name survives being domain-agnostic.

### Construction: required dependencies as parameters, config as options

Every widget is built the same way: **what it cannot work without is a parameter; what has a sane default is a variadic option.**

```go
type Option func(*Model)

func New(mark MarkFunc, load LoadFunc, opts ...Option) Model {
    m := Model{mark: mark, load: load,
        styles: DefaultStyles(), keyMap: DefaultKeyMap()}
    for _, opt := range opts {
        opt(&m)
    }
    return m
}
```

`bubbles` uses options for configuration — `viewport.New(WithWidth(80), WithHeight(24))`, `table.New(WithColumns(...))` — and that is right for configuration, because every one of those has a default and omitting it is legal.

Dependencies are different, and options are the wrong tool for them. A variadic option list **cannot express a requirement**: `New()` with no arguments compiles, and a widget that needed a `MarkFunc` discovers that at runtime as a nil dereference, in whatever session first presses the key. There is no fallback to supply either — a no-op default would silently swallow the action. Making them parameters moves the whole class of error to compile time, and makes `New`'s signature an exact statement of what the widget can do.

This replaces the six `Open(...)` signatures and the `Set*` calls that follow construction today.

### Dependencies are callables the widget declares

A widget **declares the callables it needs as named function types**, in its own package, and calls them from `Update`, returning the resulting `tea.Cmd`:

```go
package conversationlist

// Declared here, by the caller of the dependency, not by core.
type (
    MarkFunc func(convID, msgID string) tea.Cmd
    LoadFunc func(convID string, maxAge time.Duration) tea.Cmd
)

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if key.Matches(msg, m.keyMap.MarkRead) {
            return m, m.mark(m.currentID(), m.lastVisibleID())
        }
    }
    return m, nil
}
```

`ui` supplies them at construction — a method value over its `core` adapter, `conversationlist.New(a.markRead, a.loadHistory)` — and stops being in the path. This is what collapses the single-widget reducer files: the decision, the call and the result handling all live in the widget that owns the interaction.

**Why function types rather than interfaces.** A single-method interface is a function wearing a struct. Naming the function type instead gives the same decoupling with less ceremony: `ui` passes a method value or a closure with no adapter type to declare, and a test passes a literal. It is the `http.HandlerFunc` shape, and Go's convention of accepting the narrowest thing that does the job. Interfaces stay where there are genuinely several related methods to group — `Directory` below has six, a body renderer has three — and those are passed as parameters too, for the same reason.

Three properties keep this from becoming a scattering of I/O:

- **The signature is the widget's, not `core`'s.** It names what the widget wants (`MarkFunc`), not what the backend offers, which is what keeps the widget domain-agnostic.
- **It returns `tea.Cmd`, never performs I/O.** The widget stays pure; the runtime still owns execution.
- **Policy stays out.** No debounce, no retry, no quota awareness in the widget — see below.

Not every widget takes dependencies. `confirmprompt` takes none and keeps a bare `New(opts ...Option)`; `conversationlist` takes two. The signature says which, and a widget that accumulates six is telling you it does too much.

### Effects: three layers

```
component  ->  an interface it declares; states INTENT only
core       ->  cache, single-flight, rate limiting, retry, freshness
ui         ->  cross-component reconciliation only
```

Ownership follows where a result lands and whose policy governs it: landing in one component under local policy makes it the component's; landing in one component but costing shared transport makes it the component's to call and `core`'s to throttle; landing in many components makes it `App`'s to fan out.

Two subtleties a naive move breaks. **Focus gating is not rate limiting** — `pendingChannelMark` conflates *should this mark happen* (is the user looking: `terminalFocused`, `autoMarkArmed`, `inTmux`, and the tmux case is delicate) with *how often marks may go out*. The first is genuinely UI knowledge and stays; the second moves. And **the move trades free concurrency-safety for explicit locking**: those 15 fields are goroutine-safe today by construction, because only the update loop touches them. In `core` they become reachable from `Cmd` goroutines.

**Do not make the read ports implicitly cached.** If `Fetch` silently serves cache, callers lose the ability to force a refresh, which reconnect-sync and cache-verify both need. Move the *tiering decision* behind a `maxAge` argument; keep the explicit `ReadCache`/`SyncedAt`/`Fetch` triad.

### Theme: per-component `Styles`, palette in the app

Each component gets a local `Styles` and `DefaultStyles()`, set by option or `SetStyles`. `App` owns the palette and pushes a snapshot on construction and on theme change. The global dissolves.

The constraint to respect: `Apply()` + `Version()` is a working runtime theme-switch and cache-invalidation protocol — `styles.Version()` feeds the per-pane render memo used by six panes. The snapshot mechanism must land before any component stops reading the global.

### Data types: neutral, transferred by alias

The types are already neutral chat concepts; only the names are Slack (`TS` → `ID`, `ThreadTS` → `ParentID`). Go aliases let ownership invert without touching call sites:

```go
type ReadState = core.ReadState                  // step 1: component adopts it
type ReadState struct{ ... }                     // step 2: component owns it
type ReadState = conversationlist.ReadState      // core keeps an alias
```

### Slack payloads: injected renderers

`messages` and `thread` call `blockkit` at six places. Two widget-declared contracts cover all six: a body renderer, which groups three related methods (`Render`, `RenderLegacy`, `RendersBody`) and so stays an interface, and a plain-text fallback, which is one method and so is a `TextFallbackFunc`. A pane renders nothing useful without them, so both are `New` parameters; `blockkit` becomes the Slack implementation, and a backend with no rich payloads passes a plain-text pair.

### Directory: pull, not push

`App` currently fans six directory maps out to every pane and hand-replays them onto late-created windows. Replace with a read-only interface — `UserName`, `ChannelName`, `CustomEmoji`, `Avatar`, `Presence`, and one `Version()` invalidation signal instead of six pushes. Six related methods is a real interface rather than a callable, and a widget that renders names cannot work without it, so it is a `New` parameter like any other dependency.

### What stays in `ui`: reconciliation

"Reconciliation" is the one job this RFC leaves with `App`, so it is worth naming precisely. It means **holding an invariant that spans several widgets** — keeping them mutually consistent after an event none of them individually owns.

`reducer_channels.go` is the clearest case. One `ChannelSelectedMsg` must leave six widgets agreeing about which channel is open: the conversation list's selection, active id and threads-inactive flag; the message pane's channel name, type and contents; the status bar's channel, type and syncing state; both compose instances' active channel, with any open mention popup closed; and the finder's last-visited stamp. No widget can do that, because each one knows only itself. The invariant lives above them, and so does the code.

Contrast the couriering case. Marking a conversation read is one widget's intent, one service call, and a result that lands back in the same widget. Nothing else needs to know, so nothing above needs to be involved — which is exactly why it can move into the widget.

The test, applied per event: **if it leaves one widget changed, it is couriering and belongs to that widget; if it must leave several consistent, it is reconciliation and stays in `ui`.** That line is what unresolved question 2 asks to have written down before the first migration.

### Enforcement

An AST-walk test modelled on the existing `boundary_test.go`, landing first with skip-lists at today's state so it passes on day one. Every later PR shrinks a list: no sibling imports, no `debuglog`/`core` imports, no `styles.*` reads, no `func(tea.Msg)` struct fields, no geometry parameter on `View`.

The skip-list is also the coordination mechanism. A feature PR that adds a `styles.*` read to an unmigrated component passes; one that adds it to a migrated component fails. No meetings, no held branches.

## Benefits

Portability is the framing, but it is not the payoff. The payoff is that a widget becomes a thing you can change, test, measure and reason about on its own. Each of the following is measured against the tree at `4d4365b`.

### Changing behaviour

**Today** a widget cannot own an interaction. Adding a key to one means editing the app's `KeyMap`, the mode file that routes to it, the reducer that interprets the result, and the widget — and the result type is one of nine shapes, so the routing is bespoke each time. Marking a conversation read, one feature, is spread across seven non-test files including `cmd/slk/main.go`. `App` reaches into widgets at **606 call sites across 178 distinct methods**, and widgets expose **106 `Set*` methods** to receive what it pushes.

**After**, a self-contained interaction is one file: the widget's `KeyMap` gains a binding, its `Update` gains a case, and it returns the `tea.Cmd`. The 12 single-widget reducer files stop existing. The `Set*` surface shrinks to what genuinely crosses widgets, because everything else arrives at `New`.

### Fixing bugs and diagnosing them

**Today** a widget's rendered output is not a function of its arguments. It depends on `View(width, height)`, on the widget's own state, *and* on process-global theme state mutated by `styles.Apply()` — **394 reads across 23 packages**. Reproducing a rendering bug means reproducing the whole program's theme state.

Worse, `View` is not pure. Rendering pushes state into widgets at seven places (`view_messages.go:71,91,122,123`, `view_sidebar.go:42`, `view_thread.go:41,44`), plus four writes into layout — so drawing a frame can change what the next frame draws. The ordering that results is already load-bearing enough to need a comment explaining it:

> `a.messagepane.SetFocused(msgFocused)` — must run BEFORE the cache hit-check (SetFocused bumps Version on a flip and the cache key includes Version). — `view_messages.go:36`

That is a render-order invariant maintained by hand, in prose, between two files. With `SetSize` in the resize path and a pure `View`, it stops existing rather than being documented.

**After**, output is `f(Model, Styles, size)`. A bug reproduces from a literal `Model` and a pinned `Styles` in a test, with no `App`, no theme global and no ordering. "Which component is wrong" also stops being a question — the render that produced the bad cell belongs to exactly one package.

### Testing

**Today** the theme global makes component tests contaminate each other: **57 test sites call `styles.Apply()`**, each mutating shared process state. That is why **`t.Parallel()` appears zero times** anywhere in `internal/ui`. Verification is also concentrated at the top: **8 goldens, all App-level**, so a one-widget change is proven by rendering the entire application, and a golden diff does not say which widget moved.

**After**, `Styles` is a parameter, so tests pin a theme by passing one and can run in parallel. Each widget gets its own golden at a fixed size, which fails only for changes to that widget. And the feedback loop shortens by orders of magnitude: `go test ./internal/ui/` takes **6.0s**, while a component package such as `statusbar` or `confirmprompt` takes **3–4ms**. Those aren't equivalent workloads, but they are what you actually wait for — today a widget change is gated on the former.

### Benchmarking

**Today** only **5 of 29** components have a `bench_test.go`, and the two hot paths resist measurement. `Update` cannot be benchmarked on 28 of 29 widgets because it does not exist — the keystroke path is `HandleKey` returning one of nine types, driven by `App`. `View` can be called, but it reads globals, so a number depends on whatever `Apply()` ran last, and allocation counts fold in work `App` did on the widget's behalf.

**After**, both are ordinary: `Update(tea.Msg)` runs per keystroke, `View() string` per frame, each benchable with `b.ReportAllocs()` against a constructed `Model`. A regression is attributable to one widget instead of to the frame. That matters for the panes that redraw per keystroke at terminal size — `message` and `thread` — where an extra allocation per line is invisible in an app-level number.

### Navigation and reasoning

**Today** there is no way to ask what a widget can do. Its capabilities are distributed across `App`'s 3,965-line `app.go`, the mode and reducer files, and its own `Set*` surface; you answer the question by grepping for `a.<widget>.`. Nor is there a way to ask what it needs: dependencies arrive as post-construction pushes, so the only honest answer is "whatever `App` remembered to call". Six different `View` signatures and six `Open(...)` signatures mean each widget must be learned separately.

**After**, three declarations answer it. `New`'s signature is the exhaustive list of what the widget needs — the compiler will not let it be otherwise. Its `KeyMap` is the exhaustive list of keys it handles. Its exported `tea.Msg` types are the exhaustive list of what it can tell the outside world. Every widget has the same shape, so learning the second is free, and a contributor who knows `bubbles` already knows it.

### Review and blast radius

**Today** the unit of change is `App`. A widget edit lands in files that every other feature branch also edits — `app.go`, `mode_normal.go`, `reducer_channels.go` — so unrelated work conflicts, and a reviewer cannot tell from the diff whether anything outside the widget moved.

**After**, the unit of change is a package. The boundary test makes "nothing outside this widget moved" a mechanical property rather than a reviewer's judgement, and two people working on two widgets stop rebasing onto each other.

### What this does not fix

`App` keeps the 14 fan-out reducer files, and they are the highest-churn files in the repo. Shrinking `App` is a consequence of this work, not its purpose, and `mode_normal.go` will still touch 12 widgets afterwards because mode arbitration genuinely spans them.

## Drawbacks

- **A long campaign.** Most PRs are mechanical and under 100 lines, but a half-finished migration leaves two conventions in the tree at once.
- **Concurrency risk concentrates in one stage.** Moving transport policy into `core` makes that state concurrently reachable, and three data races of exactly this closure-capture shape already exist in `cmd/slk/main.go` (`activeTeamID`, `workspaces`, and a `cfg` copy aliasing its `Workspaces` map). The locking has to be designed, not inherited.
- **Widgets holding services is a real loosening.** Today there is exactly one place that can call a service, which makes the call graph trivially auditable. Afterwards there are many, and "narrow signature, returns `tea.Cmd`, no policy" has to be enforced by review and by the boundary test rather than by construction.
- **`messages`/`thread` convergence stays risky.** 377 verbatim shared lines and a lockstep test with 15 documented divergences that fires *on* convergence.
- **Some churn buys slk nothing directly.** Neutral field names make components self-contained; they make slk marginally more indirect.

## Alternatives

**Do nothing.** The coupling is not hurting slk today. Rejected because split view already needs two differently-styled instances of one component, which the global forbids, and because transport policy sitting in the presentation layer is a correctness problem independent of any extraction.

**Extract to `pkg/` now.** Rejected as premature: it implies a stability promise before the conventions have been proven on more than one widget, and it makes every intermediate state a public-API question. `internal/` first, reconsider later.

**Keep `App` as the only service caller** and extract components for styling and keymaps only. Cheaper and preserves the audit property above. Rejected because it leaves the 12 single-widget reducer files in place. Those are a minority of the reducer files, but they are the ones whose logic most obviously belongs to a widget, and leaving them means components stay tidier-but-inert: still unable to express an intention without `App` translating it.

**Pass dependencies as options too**, for a uniform `New(...Option)` everywhere. Rejected: a variadic list cannot express a requirement, so a missing dependency becomes a nil dereference at the first keypress instead of a compile error, and there is no honest default to fall back on — a no-op would swallow the action silently. The cost of the split is that adding a required dependency breaks every call site, which for a required dependency is the correct outcome.

**Keep interfaces for every dependency**, including single-method ones. More uniform, and marginally easier to extend from one method to two. Rejected as ceremony: it makes `ui` declare an adapter type per widget where a method value would do, and it obscures that most of these dependencies are one function.

**An event bus** replacing `App`'s mediation. Rejected: 11 lines of cross-component traffic do not justify losing the explicit call graph.

## Precedent

Nothing here is novel; it is the shape upstream already converged on.

**`charmbracelet/bubbles`** is the reference for all of it, and its v2 upgrade is the strongest single argument for the theme change. From `UPGRADE_GUIDE_V2.md`:

- *"Global mutable `DefaultKeyMap` variables have been replaced with functions that return fresh values"* — `paginator`, `textarea`, `textinput`. Upstream hit the same package-global problem and resolved it the way this RFC proposes.
- Scattered style fields collapsed into one `Styles`, built by `DefaultStyles(isDark)`, swapped with `SetStyles()`.
- `KeyMap` as a field on the `Model`, defaulted by `DefaultKeyMap()`.
- The component owns its size: `SetWidth()`, not geometry threaded through `View`.

Options are the same `func(*Model)` this RFC uses for configuration — `viewport.New(WithWidth(80), WithHeight(24))`, `table.New(WithColumns(...))`:

```go
func WithColumns(cols []Column) Option {
    return func(m *Model) { m.cols = cols }
}
```

bubbles also puts required arguments in the signature rather than in options where a component genuinely has them: `list.New(items, delegate, width, height)` takes its four up front. This RFC applies that split consistently — required dependency in the signature, optional configuration in an option.

**`charmbracelet/huh`** is the precedent for where a theme lives once it is not a global: `Theme` is an interface, `Theme(isDark bool) *Styles`, injected with `WithTheme` and propagated form → group → field, with nested `FormStyles` / `GroupStyles` / `FieldStyles`. The owner supplies it; the component holds the resulting `Styles`. huh's "first theme wins" rule is also a reminder that propagation order is a decision, not just a push.

## Unresolved questions

1. **`viewport` or hand-rolled `yOffset`?** `thread` uses `bubbles/viewport`, `messages` hand-rolls it. Sets the pane substrate's shape and blocks the fork collapse.
2. **How far does "the widget calls its own services" go?** Marking read and searching are clearly the widget's. Loading history is shared transport. Workspace switching is plainly `App`'s. The boundary needs to be written down before the first migration, or it will be litigated per PR.
3. **How neutral should presence be?** `peerstatus` models DND and huddles as first-class. A neutral `Presence` with an escape hatch, or a component parameterised over a presence interface?
4. **One pane with a mode, or two panes sharing a substrate**, for `messages`/`thread`?
5. **Does `emoji` (5,232 lines) belong under `internal/bubbles/`** or beside it? Shortcode maps are universal to chat TUIs; the custom-emoji fetch is not.

## Future possibilities

Once the conventions hold across the component set and the enforcement test's skip-lists are empty, the packages under `internal/bubbles/` are by construction importable by something other than slk. Moving some of them to `pkg/` — or to a separate module — becomes a mechanical import rewrite plus a decision about supporting external users. That is deliberately out of scope here: it should be argued on evidence from the finished in-tree state, not promised up front.

## References

- Bubbles v2 upgrade guide — `https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md`
- Bubbles `table` / `viewport` construction options — `https://github.com/charmbracelet/bubbles`
- Huh theming (`Theme`, `WithTheme`, `Styles`) — `https://github.com/charmbracelet/huh`
