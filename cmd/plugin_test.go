// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPlugins(t *testing.T) {
	// Create a temporary directory for test plugins
	tmpDir, err := os.MkdirTemp("", "checkpointctl-plugin-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a fake plugin executable
	pluginPath := filepath.Join(tmpDir, "checkpointctl-testplugin")
	if err := os.WriteFile(pluginPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test plugin: %v", err)
	}

	// Create a non-executable file (should be ignored)
	nonExecPath := filepath.Join(tmpDir, "checkpointctl-nonexec")
	if err := os.WriteFile(nonExecPath, []byte("not executable"), 0o644); err != nil {
		t.Fatalf("Failed to create non-exec file: %v", err)
	}

	// Create a file without the prefix (should be ignored)
	otherPath := filepath.Join(tmpDir, "other-binary")
	if err := os.WriteFile(otherPath, []byte("#!/bin/sh\necho other"), 0o755); err != nil {
		t.Fatalf("Failed to create other binary: %v", err)
	}

	// Create a directory with the prefix (should be ignored)
	dirPath := filepath.Join(tmpDir, "checkpointctl-dir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Prepend temp dir to PATH
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

	// Discover plugins
	plugins := DiscoverPlugins()

	// Find our test plugin
	var found bool
	for _, p := range plugins {
		if p.Name == "testplugin" {
			found = true
			if p.Path != pluginPath {
				t.Errorf("Expected path %s, got %s", pluginPath, p.Path)
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find testplugin, but it was not discovered")
	}

	// Verify non-executable and other files were not discovered
	for _, p := range plugins {
		if p.Name == "nonexec" {
			t.Error("Non-executable file should not be discovered as plugin")
		}
		if p.Name == "dir" {
			t.Error("Directory should not be discovered as plugin")
		}
	}
}

func TestDiscoverPluginsFirstInPathWins(t *testing.T) {
	// Create two temporary directories
	tmpDir1, err := os.MkdirTemp("", "checkpointctl-plugin-test1")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "checkpointctl-plugin-test2")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tmpDir2)

	// Create same plugin in both directories
	plugin1 := filepath.Join(tmpDir1, "checkpointctl-dupe")
	plugin2 := filepath.Join(tmpDir2, "checkpointctl-dupe")
	if err := os.WriteFile(plugin1, []byte("#!/bin/sh\necho first"), 0o755); err != nil {
		t.Fatalf("Failed to create plugin 1: %v", err)
	}
	if err := os.WriteFile(plugin2, []byte("#!/bin/sh\necho second"), 0o755); err != nil {
		t.Fatalf("Failed to create plugin 2: %v", err)
	}

	// Set PATH with tmpDir1 first
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir1+string(os.PathListSeparator)+tmpDir2)
	defer os.Setenv("PATH", originalPath)

	plugins := DiscoverPlugins()

	var foundCount int
	for _, p := range plugins {
		if p.Name == "dupe" {
			foundCount++
			if p.Path != plugin1 {
				t.Errorf("Expected first plugin in PATH (%s), got %s", plugin1, p.Path)
			}
		}
	}

	if foundCount != 1 {
		t.Errorf("Expected exactly 1 'dupe' plugin, found %d", foundCount)
	}
}

func TestCreatePluginCommandWithDescription(t *testing.T) {
	plugin := Plugin{
		Name:        "testcmd",
		Path:        "/usr/bin/checkpointctl-testcmd",
		Description: "My custom description",
	}

	cmd := CreatePluginCommand(plugin)

	if cmd.Use != "testcmd" {
		t.Errorf("Expected Use to be 'testcmd', got '%s'", cmd.Use)
	}

	if cmd.Short != "My custom description" {
		t.Errorf("Expected Short to be 'My custom description', got '%s'", cmd.Short)
	}

	if !cmd.DisableFlagParsing {
		t.Error("Expected DisableFlagParsing to be true")
	}
}

func TestCreatePluginCommandWithoutDescription(t *testing.T) {
	plugin := Plugin{
		Name:        "testcmd",
		Path:        "/usr/bin/checkpointctl-testcmd",
		Description: "",
	}

	cmd := CreatePluginCommand(plugin)

	if cmd.Use != "testcmd" {
		t.Errorf("Expected Use to be 'testcmd', got '%s'", cmd.Use)
	}

	expected := "Plugin provided by /usr/bin/checkpointctl-testcmd"
	if cmd.Short != expected {
		t.Errorf("Expected Short to be '%s', got '%s'", expected, cmd.Short)
	}

	if !cmd.DisableFlagParsing {
		t.Error("Expected DisableFlagParsing to be true")
	}
}
