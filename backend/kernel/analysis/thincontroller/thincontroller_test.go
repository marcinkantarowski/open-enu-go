package thincontroller

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// The fixture is a module that does everything api.go must not; every
// `// want` is one rule proved to fire.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "example.com/app/internal/module/demo/internal")
}
