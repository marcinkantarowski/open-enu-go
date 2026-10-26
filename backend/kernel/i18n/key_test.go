package i18n

import "testing"

func TestKeyShape(t *testing.T) {
	valid := []string{"kernel.not_found", "example.project.created", "a.b1_c.d"}
	invalid := []string{"", "kernel", "Kernel.x", "kernel..x", "kernel.x.", "a sentence", "kernel.Not"}
	for _, s := range valid {
		if !Key(s).Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range invalid {
		if Key(s).Valid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestMustKeyPanicsOnASentence(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustKey accepted a sentence")
		}
	}()
	MustKey("Project not found.")
}
