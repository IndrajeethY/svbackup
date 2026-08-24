package retention

import (
	"fmt"
	"sort"
	"time"
)

type Snapshot struct {
	Name string
	Time time.Time
}

const layout = "2006-01-02_1504"

func Parse(name string) (Snapshot, bool) {
	t, err := time.ParseInLocation(layout, name, time.Local)
	if err != nil {
		return Snapshot{}, false
	}
	return Snapshot{Name: name, Time: t}, true
}

func Plan(snaps []Snapshot, daily, weekly int) (keep, del []Snapshot) {
	if len(snaps) == 0 {
		return nil, nil
	}
	sorted := make([]Snapshot, len(snaps))
	copy(sorted, snaps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.After(sorted[j].Time) })

	keepSet := map[string]bool{}

	seenDay := map[string]bool{}
	dailyDays := map[string]bool{}
	for _, s := range sorted {
		day := s.Time.Format("2006-01-02")
		if seenDay[day] {
			continue
		}
		seenDay[day] = true
		if len(dailyDays) < daily {
			keepSet[s.Name] = true
			dailyDays[day] = true
		}
	}

	weeklyWeeks := map[string]bool{}
	seenWeek := map[string]bool{}
	for _, s := range sorted {
		day := s.Time.Format("2006-01-02")
		if dailyDays[day] {
			continue
		}
		y, w := s.Time.ISOWeek()
		wk := isoKey(y, w)
		if seenWeek[wk] {
			continue
		}
		seenWeek[wk] = true
		if len(weeklyWeeks) < weekly {
			keepSet[s.Name] = true
			weeklyWeeks[wk] = true
		}
	}

	for _, s := range sorted {
		if keepSet[s.Name] {
			keep = append(keep, s)
		} else {
			del = append(del, s)
		}
	}
	return keep, del
}

func isoKey(y, w int) string {
	return fmt.Sprintf("%04d-W%02d", y, w)
}
