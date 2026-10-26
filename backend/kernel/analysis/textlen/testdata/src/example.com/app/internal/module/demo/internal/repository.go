package internal

// Only api.go and handler.go check input; elsewhere a length is whatever it is.
func short(id string) bool { return len(id) <= 32 }
