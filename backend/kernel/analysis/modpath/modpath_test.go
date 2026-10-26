package modpath

import "testing"

func TestModule(t *testing.T) {
	root, name, rest, ok := Module("github.com/acme/app/internal/module/billing/internal")
	if !ok || root != "github.com/acme/app/internal/module/billing" || name != "billing" || rest != "internal" {
		t.Errorf("got %q %q %q %v", root, name, rest, ok)
	}
	if _, _, _, ok := Module("github.com/acme/app/cmd/openenu"); ok {
		t.Error("cmd is not a module")
	}
	if _, _, rest, _ := Module("x/internal/module/a"); rest != "" {
		t.Errorf("module root has empty rest, got %q", rest)
	}
	if !IsKernel("github.com/open-enu/kernel/httpx") || IsKernel("github.com/open-enu/kernelx") {
		t.Error("IsKernel")
	}
}
