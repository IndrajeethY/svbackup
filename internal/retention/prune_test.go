package retention

import (
	"testing"
	"time"
)

func mk(names ...string) []Snapshot {
	var out []Snapshot
	for _, n := range names {
		s, ok := Parse(n)
		if !ok {
			panic("bad test snapshot name: " + n)
		}
		out = append(out, s)
	}
	return out
}

func names(ss []Snapshot) map[string]bool {
	m := map[string]bool{}
	for _, s := range ss {
		m[s.Name] = true
	}
	return m
}

func TestPlanDailyWindow(t *testing.T) {
	var in []Snapshot
	base := time.Date(2026, 8, 19, 3, 30, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		d := base.AddDate(0, 0, -i)
		in = append(in, Snapshot{Name: d.Format(layout), Time: d})
	}
	keep, del := Plan(in, 7, 0)
	if len(keep) != 7 {
		t.Fatalf("want 7 kept, got %d", len(keep))
	}
	if len(del) != 3 {
		t.Fatalf("want 3 deleted, got %d", len(del))
	}
}

func TestPlanKeepsNewestPerDay(t *testing.T) {
	in := mk("2026-08-19_0330", "2026-08-19_1200", "2026-08-18_0330")
	keep, del := Plan(in, 7, 0)
	k := names(keep)
	if !k["2026-08-19_1200"] || !k["2026-08-18_0330"] {
		t.Fatalf("expected newest-of-day + prior day kept, got %v", k)
	}
	if len(del) != 1 || del[0].Name != "2026-08-19_0330" {
		t.Fatalf("expected older same-day pruned, got del=%v", names(del))
	}
}

func TestPlanWeekly(t *testing.T) {
	in := mk(
		"2026-08-19_0330", "2026-08-18_0330", "2026-08-17_0330",
		"2026-08-10_0330",
		"2026-08-03_0330",
		"2026-07-27_0330",
	)
	keep, _ := Plan(in, 3, 2)
	k := names(keep)
	if !k["2026-08-10_0330"] || !k["2026-08-03_0330"] {
		t.Fatalf("expected 2 weekly kept, got %v", k)
	}
	if k["2026-07-27_0330"] {
		t.Fatalf("week -3 should have been pruned, got %v", k)
	}
	if len(keep) != 5 {
		t.Fatalf("want 3 daily + 2 weekly = 5 kept, got %d", len(keep))
	}
}

func TestParseRejectsJunk(t *testing.T) {
	if _, ok := Parse("not-a-timestamp"); ok {
		t.Fatal("expected junk to be rejected")
	}
}
