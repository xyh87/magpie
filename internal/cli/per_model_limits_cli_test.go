package cli

import (
	"strings"
	"testing"
)

// magpie provider per-model-limits <id> on|off sets the provider's own
// switch, and provider set takes the same setting as a k=v pair.
func TestProviderPerModelLimits(t *testing.T) {
	groupsHome(t)
	if _, err := printed(t, func() error { return providerCmd([]string{"provider", "per-model-limits", "a", "on"}) }); err != nil {
		t.Fatal(err)
	}
	if p := mustFind(t, "a"); !p.PerModelLimits {
		t.Fatal("per-model-limits on didn't save")
	}
	if _, err := printed(t, func() error { return providerCmd([]string{"provider", "set", "a", "per-model-limits=no"}) }); err != nil {
		t.Fatal(err)
	}
	if p := mustFind(t, "a"); p.PerModelLimits {
		t.Fatal("per-model-limits=no left it on")
	}
	if err := providerCmd([]string{"provider", "per-model-limits", "a", "maybe"}); err == nil || !strings.Contains(err.Error(), "on|off") {
		t.Fatalf("per-model-limits maybe: %v", err)
	}
	if p := mustFind(t, "a"); p.PerModelLimits {
		t.Fatal("a refused value turned it on")
	}
}
