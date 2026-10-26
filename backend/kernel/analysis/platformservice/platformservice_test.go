package platformservice

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer,
		"example.com/app/internal/module/demo/internal", // reported
		"github.com/open-enu/kernel/storage",            // the kernel may; nothing reported
	)
}
