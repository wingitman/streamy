package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wingitman/streamy/internal/config"
)

type Result struct {
	Available       bool
	Current, Latest string
	Error           error
}

func Check(cfg config.Updates, current string) Result {
	r := Result{Current: current}
	if cfg.DisableChecks || cfg.Repository == "" {
		return r
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(cfg.Repository, "/"), "https://github.com/"), "/")
	if len(parts) < 2 {
		r.Error = fmt.Errorf("invalid update repository %q", cfg.Repository)
		return r
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+parts[0]+"/"+parts[1]+"/commits?per_page=1", nil)
	if err != nil {
		r.Error = err
		return r
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		r.Error = err
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.Error = fmt.Errorf("update check returned %s", resp.Status)
		return r
	}
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		r.Error = err
		return r
	}
	if len(commits) > 0 {
		r.Latest = commits[0].SHA
		r.Available = r.Latest != current && !strings.HasPrefix(r.Latest, current)
	}
	return r
}
