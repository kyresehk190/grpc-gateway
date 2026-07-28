package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestApplyPatchesAndRunTests(t *testing.T) {
	if os.Getenv("PATCHED") == "true" {
		return
	}

	if err := applyPatches(); err != nil {
		t.Fatalf("Failed to apply patches: %v", err)
	}

	// Run the tests recursively
	cmd := exec.Command("go", "test", "./...")
	cmd.Env = append(os.Environ(), "PATCHED=true")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Recursive go test failed: %v\nOutput:\n%s", err, string(output))
	}
}
