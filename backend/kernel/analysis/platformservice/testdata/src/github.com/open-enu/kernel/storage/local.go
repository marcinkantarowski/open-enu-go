package storage

import "os"

func put(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }
