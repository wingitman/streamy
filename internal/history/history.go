package history

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/wingitman/streamy/internal/config"
)

type Entry struct {
	At      time.Time `toml:"at"`
	Label   string    `toml:"label"`
	Command string    `toml:"command"`
}

func (e Entry) String() string {
	return fmt.Sprintf("%s  %s  %s", e.At.Format(time.RFC3339), e.Label, e.Command)
}
func Load(cfg config.History) ([]Entry, error) {
	if !cfg.Enabled || cfg.File == "" {
		return nil, nil
	}
	var file struct {
		Entries []Entry `toml:"entries"`
	}
	if _, err := toml.DecodeFile(cfg.File, &file); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return file.Entries, nil
}
func Append(cfg config.History, entry Entry) error {
	if !cfg.Enabled || cfg.File == "" {
		return nil
	}
	entries, err := Load(cfg)
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	max := cfg.MaxEntries
	if max < 1 {
		max = 100
	}
	if len(entries) > max {
		entries = entries[len(entries)-max:]
	}
	if err := os.MkdirAll(filepath.Dir(cfg.File), 0750); err != nil {
		return err
	}
	data, err := toml.Marshal(struct {
		Entries []Entry `toml:"entries"`
	}{entries})
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.File, data, 0640)
}
