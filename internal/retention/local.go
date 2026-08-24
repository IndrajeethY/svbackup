package retention

import (
	"os"
	"path/filepath"
)

func PruneLocal(snapshotsRoot string, daily, weekly int) ([]string, error) {
	entries, err := os.ReadDir(snapshotsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snaps []Snapshot
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if s, ok := Parse(e.Name()); ok {
			snaps = append(snaps, s)
		}
	}
	_, del := Plan(snaps, daily, weekly)
	var deleted []string
	for _, s := range del {
		if err := os.RemoveAll(filepath.Join(snapshotsRoot, s.Name)); err != nil {
			return deleted, err
		}
		deleted = append(deleted, s.Name)
	}
	return deleted, nil
}
