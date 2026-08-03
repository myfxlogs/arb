# Example: Push Notification Module — Before vs After

Real case from AntClaw `backend/cmd/antclaw-worker/`.

## Before (first-pass implementation)

Four files, each with its own copy of:

```go
// ---- push_calendar.go (407 lines) ----
// Duplicated in every push_*.go:
type alertPrefs struct { Currencies []string; ... }              // def #1
func userAlertPrefsFromRow(r) alertPrefs { ... }                 // def #1
func (p alertPrefs) matchesCurrency(c string) bool { ... }       // def #1
func (p alertPrefs) matchesImpact(impact string) bool { ... }    // def #1

func pushCalendar(env) {
    var cursor uuid.UUID
    for {                                       // pattern: scanUsers × 12
        users, _ := e.q.ListUsersWithAlertPrefs(...)
        if len(users) == 0 { break }
        for _, u := range users {
            cursor = u.UserID
            prefs := userAlertPrefsFromRow(u)
            // ... business logic ...
        }
        if len(users) < 100 { break }
    }
}

func tryPreEventNotify(...) {
    if e.alreadyPushed(...) { return 0 }        // pattern: sendIfNotPushed × 30
    // ... build notification ...
    if err := e.svc.Send(...); err != nil { ... }
    e.recordPush(...)
}

// ---- push_digest.go (453 lines) ----
// Same alertPrefs, same methods (def #2), same scan loop (×4 more)

func countTodayEvents(ctx, prefs) { ... }       // 80% same as countWeekEvents
func countWeekEvents(ctx, prefs) { ... }        // 80% same as countTodayEvents
```

**Problems:**
- `alertPrefs` type defined in 2 files (duplicate)
- `scanUsers` for-loop repeated 12 places
- `alreadyPushed→Send→recordPush` template 30+ places
- `countTodayEvents`/`countWeekEvents` 80% identical
- `dayRange`/`weekRange` logic duplicated inline
- Total: **1861 lines**

## After (extracting push_util.go)

```go
// ---- push_util.go (320 lines, shared) ----

type alertPrefs struct { ... }              // single definition
func userAlertPrefsFromRow(r) alertPrefs { ... }

func (e *pushEnv) scanUsers(ctx, fn) {     // eliminates 12 copies
    var cursor uuid.UUID
    for {
        users, _ := e.q.ListUsersWithAlertPrefs(...)
        if len(users) == 0 { return }
        for _, u := range users {
            cursor = u.UserID
            fn(u.UserID, userAlertPrefsFromRow(u))
        }
        if len(users) < 100 { return }
    }
}

func (e *pushEnv) sendIfNotPushed(ctx, uid, key, typ, n) bool {
    if e.alreadyPushed(...) { return false }   // eliminates 30+ copies
    n.DedupKey = key
    if err := e.svc.Send(ctx, n); err != nil { return false }
    e.recordPush(...)
    return true
}

func (e *pushEnv) countEventsInRange(ctx, start, end, currencies) (counts, first string) {
    // merged countTodayEvents + countWeekEvents
}

func dayRange(p alertPrefs) (start, end time.Time) { ... }
func weekRange(p alertPrefs) (start, end time.Time) { ... }

// ---- push_calendar.go (172 lines, business only) ----

func pushCalendar(env) {
    events, _ := e.scanCalendarEvents(...)
    e.scanUsers(ctx, func(uid, p) {
        for _, evt := range events {
            sentPre += e.tryPreEventNotify(ctx, uid, evt, p)
        }
    })
}

func tryPreEventNotify(...) {
    return e.sendIfNotPushed(ctx, uid, ek, "calendar_pre", &notify.Notification{...})
}
```

**Results:**
- Total: **1178 lines** (-37%)
- `scanUsers` loop: 12 copies → 1
- `sendIfNotPushed` template: 30+ copies → 1
- `countTodayEvents`/`countWeekEvents`: 2 → 1 merged function
- All business files now purely business logic, no infrastructure duplication
