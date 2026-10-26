package internal

import (
	"os"

	"github.com/redis/rueidis" // want `github.com/redis/rueidis imported outside the kernel`
)

var _ rueidis.Client

func export(path string, data []byte) error {
	if err := os.MkdirAll("/tmp/x", 0o755); err != nil { // want `os.MkdirAll outside the kernel`
		return err
	}
	_, _ = os.ReadFile(path)               // reading is fine
	return os.WriteFile(path, data, 0o644) // want `os.WriteFile outside the kernel - files go through kernel/storage`
}
