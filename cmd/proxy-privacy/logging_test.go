package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupLogResources_TraceDisabled(t *testing.T) {
	t.Parallel()

	resources, err := setupLogResources(false, "")
	if err != nil {
		t.Fatal(err)
	}

	if resources.traceLogger != nil {
		t.Error("traceLogger should be nil when trace is disabled")
	}

	if resources.cleanup != nil {
		resources.cleanup()
	}
}

func TestSetupLogResources_TraceEnabledCreatesFiles(t *testing.T) {
	dir := t.TempDir()

	resources, err := setupLogResources(true, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer resources.cleanup()

	if resources.traceLogger == nil {
		t.Fatal("traceLogger should not be nil")
	}

	if _, err := os.Stat(filepath.Join(dir, "proxy-privacy.log")); os.IsNotExist(err) {
		t.Error("proxy-privacy.log was not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy-privacy.trace.log")); os.IsNotExist(err) {
		t.Error("proxy-privacy.trace.log was not created")
	}
}

func TestSetupLogResources_TraceEnabledNoDirUsesCwd(t *testing.T) {
	resources, err := setupLogResources(true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if resources.cleanup != nil {
			resources.cleanup()
		}
		os.Remove("proxy-privacy.log")
		os.Remove("proxy-privacy.trace.log")
	}()

	if resources.traceLogger == nil {
		t.Fatal("traceLogger should not be nil")
	}
}

func TestSetupLogResources_CleanupIsSafe(t *testing.T) {
	dir := t.TempDir()

	resources, err := setupLogResources(true, dir)
	if err != nil {
		t.Fatal(err)
	}

	resources.cleanup()
	resources.cleanup()
}

func TestSetupLogResources_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")

	resources, err := setupLogResources(true, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer resources.cleanup()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("setupLogResources did not create the trace directory")
	}
}

func TestSetupLogResources_InvalidDir(t *testing.T) {
	dir := t.TempDir()
	block := filepath.Join(dir, "block")
	if err := os.WriteFile(block, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := setupLogResources(true, filepath.Join(block, "subdir"))
	if err == nil {
		t.Error("expected error for path with file in the way, got nil")
	}
}

func TestSetupLogResources_TraceDisabledCleanupIsSafe(t *testing.T) {
	t.Parallel()

	resources, err := setupLogResources(false, "")
	if err != nil {
		t.Fatal(err)
	}

	if resources.cleanup != nil {
		resources.cleanup()
	}
}
