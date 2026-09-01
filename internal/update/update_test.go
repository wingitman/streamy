package update

import (
	"github.com/wingitman/streamy/internal/config"
	"testing"
)

func TestCheckSkipsDisabledUpdates(t *testing.T) {
	cfg := config.Default()
	cfg.Updates.DisableChecks = true
	result := Check(cfg.Updates, "abc")
	if result.Available || result.Error != nil || result.Current != "abc" {
		t.Fatalf("unexpected disabled result: %+v", result)
	}
}
