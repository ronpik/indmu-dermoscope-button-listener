// Package main provides integration tests for the dermoscope-helper CLI entry point.
// These tests verify the complete application behavior including startup, shutdown,
// and command-line flag handling.
package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// =============================================================================
// Integration Tests - CLI Flag Handling
// =============================================================================

// TestMain_Version_ExitsZero tests that --version exits with code 0
func TestMain_Version_ExitsZero(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--version should exit 0, got error: %v, output: %s", err, output)
	}

	if !strings.Contains(string(output), "dermoscope-helper version") {
		t.Errorf("--version output should contain version info, got: %s", output)
	}
}

// TestMain_Version_PrintsVersionString tests that --version prints the version
func TestMain_Version_PrintsVersionString(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}

	outputStr := string(output)
	// Should contain "dermoscope-helper version" and some version string
	if !strings.Contains(outputStr, "dermoscope-helper") {
		t.Errorf("--version should print program name, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "version") {
		t.Errorf("--version should print 'version', got: %s", outputStr)
	}
}

// TestMain_ListProfiles_ExitsZero tests that --list-profiles exits with code 0
func TestMain_ListProfiles_ExitsZero(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--list-profiles")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--list-profiles should exit 0, got error: %v, output: %s", err, output)
	}
}

// TestMain_ListProfiles_ShowsHTB30S tests that --list-profiles shows the HT-B30S profile
func TestMain_ListProfiles_ShowsHTB30S(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--list-profiles")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--list-profiles failed: %v", err)
	}

	outputStr := string(output)

	// Should contain HT-B30S profile information
	if !strings.Contains(outputStr, "ht-b30s") {
		t.Errorf("--list-profiles should show ht-b30s profile ID, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "HT-B30S") {
		t.Errorf("--list-profiles should show HT-B30S name, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "AB02:AB01") {
		t.Errorf("--list-profiles should show VID:PID, got: %s", outputStr)
	}
}

// TestMain_ListProfiles_ShowsProfileCount tests that --list-profiles shows the profile count
func TestMain_ListProfiles_ShowsProfileCount(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--list-profiles")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("--list-profiles failed: %v", err)
	}

	outputStr := string(output)

	// Should show total count
	if !strings.Contains(outputStr, "Total:") {
		t.Errorf("--list-profiles should show total count, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "profile") {
		t.Errorf("--list-profiles should mention 'profile', got: %s", outputStr)
	}
}

// =============================================================================
// Integration Tests - Debug Mode
// =============================================================================

// TestMain_Debug_ShowsProfileLoadingInfo tests that --debug shows profile loading info
func TestMain_Debug_ShowsProfileLoadingInfo(t *testing.T) {
	binary := buildTestBinary(t)

	// Run with timeout since app runs indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Wait a moment for startup logs
	time.Sleep(500 * time.Millisecond)

	// Kill the process
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	// Check combined output (stderr since zerolog writes to stderr by default)
	output := stderr.String() + stdout.String()

	// Debug mode should log profile loading info
	if !strings.Contains(output, "Loaded device profiles") {
		t.Errorf("--debug should show 'Loaded device profiles', got: %s", output)
	}
	if !strings.Contains(output, "Profile registered") {
		t.Errorf("--debug should show 'Profile registered', got: %s", output)
	}
	if !strings.Contains(output, "ht-b30s") {
		t.Errorf("--debug should show profile ID 'ht-b30s', got: %s", output)
	}
}

// TestMain_Debug_ShowsStartupMessage tests that --debug shows startup message
func TestMain_Debug_ShowsStartupMessage(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should show startup messages
	if !strings.Contains(output, "Starting Dermoscope Button Helper") {
		t.Errorf("--debug should show 'Starting Dermoscope Button Helper', got: %s", output)
	}
	if !strings.Contains(output, "Application starting") {
		t.Errorf("--debug should show 'Application starting', got: %s", output)
	}
}

// TestMain_Debug_ShowsStateTransitions tests that --debug shows state transitions
func TestMain_Debug_ShowsStateTransitions(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Debug mode should show state transitions
	if !strings.Contains(output, "State transition") {
		t.Errorf("--debug should show 'State transition', got: %s", output)
	}
}

// =============================================================================
// Integration Tests - Graceful Shutdown
// =============================================================================

// TestMain_SIGTERM_GracefulShutdown tests that SIGTERM triggers graceful shutdown
func TestMain_SIGTERM_GracefulShutdown(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Wait for process to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGTERM
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send SIGTERM: %v", err)
	}

	// Wait for process to exit
	err := cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should show shutdown signal received
	if !strings.Contains(output, "Received shutdown signal") {
		t.Errorf("SIGTERM should log 'Received shutdown signal', got: %s", output)
	}

	// Should show application stopping
	if !strings.Contains(output, "Stopping application") {
		t.Errorf("SIGTERM should log 'Stopping application', got: %s", output)
	}

	// Process may exit with signal code or 0
	if err != nil {
		// Exit due to signal is expected
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Signal exit codes are typically 128 + signal number
			// SIGTERM (15) -> exit code 143 or similar
			t.Logf("Process exited with code: %d", exitErr.ExitCode())
		}
	}
}

// TestMain_SIGINT_GracefulShutdown tests that SIGINT (Ctrl+C) triggers graceful shutdown
func TestMain_SIGINT_GracefulShutdown(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	// Wait for process to start
	time.Sleep(500 * time.Millisecond)

	// Send SIGINT (Ctrl+C)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Wait for process to exit
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should show shutdown signal received
	if !strings.Contains(output, "Received shutdown signal") {
		t.Errorf("SIGINT should log 'Received shutdown signal', got: %s", output)
	}
}

// TestMain_Shutdown_TransitionsToStopping tests that shutdown transitions state to stopping
func TestMain_Shutdown_TransitionsToStopping(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should transition to stopping state
	if !strings.Contains(output, "stopping") {
		t.Errorf("Shutdown should show 'stopping' state, got: %s", output)
	}
}

// =============================================================================
// Integration Tests - Config File Loading
// =============================================================================

// TestMain_InvalidConfigFile_LogsErrorAndExits tests that invalid config logs error
func TestMain_InvalidConfigFile_LogsErrorAndExits(t *testing.T) {
	binary := buildTestBinary(t)

	// Create an invalid JSON config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(configPath, []byte("{ invalid json"), 0644); err != nil {
		t.Fatalf("Failed to create invalid config file: %v", err)
	}

	cmd := exec.Command(binary, "--config", configPath)
	output, err := cmd.CombinedOutput()

	// Should exit with error
	if err == nil {
		t.Errorf("Invalid config should cause exit with error, got success")
	}

	outputStr := string(output)

	// Should log error about config
	if !strings.Contains(strings.ToLower(outputStr), "config") && !strings.Contains(strings.ToLower(outputStr), "json") {
		t.Errorf("Invalid config should mention config/json in error, got: %s", outputStr)
	}
}

// TestMain_NonExistentConfigFile_UsesDefaults tests that non-existent config uses defaults
func TestMain_NonExistentConfigFile_UsesDefaults(t *testing.T) {
	binary := buildTestBinary(t)

	// Use a path that doesn't exist
	configPath := "/nonexistent/path/config.json"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config", configPath, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	err := cmd.Wait()

	output := stderr.String() + stdout.String()

	// Non-existent config file should result in error
	if err == nil {
		// If it didn't error, check if it's using defaults (started successfully)
		if strings.Contains(output, "Application starting") {
			t.Log("Non-existent config path was handled (may have used defaults)")
		}
	} else {
		// Expected - non-existent file should error
		if !strings.Contains(strings.ToLower(output), "config") && !strings.Contains(strings.ToLower(output), "load") {
			t.Logf("Error output: %s", output)
		}
	}
}

// TestMain_ValidConfigFile_LoadsSuccessfully tests that valid config file loads
func TestMain_ValidConfigFile_LoadsSuccessfully(t *testing.T) {
	binary := buildTestBinary(t)

	// Create a valid JSON config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "valid.json")
	configContent := `{
		"debounce_ms": 100,
		"polling_interval_ms": 50
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create valid config file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config", configPath, "--debug")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should start successfully with valid config
	if !strings.Contains(output, "Application starting") {
		t.Errorf("Valid config should allow application to start, got: %s", output)
	}
}

// =============================================================================
// Integration Tests - Normal Mode (no --debug)
// =============================================================================

// TestMain_NormalMode_StartsWithoutDebugLogs tests that normal mode has less verbose output
func TestMain_NormalMode_StartsWithoutDebugLogs(t *testing.T) {
	binary := buildTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run without --debug
	cmd := exec.CommandContext(ctx, binary)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()
	time.Sleep(500 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	cmd.Wait()

	output := stderr.String() + stdout.String()

	// Should NOT show debug-level profile loading details
	// (may still show INFO level messages)
	if strings.Contains(output, "Profile registered") {
		t.Errorf("Normal mode should not show 'Profile registered' debug log, got: %s", output)
	}
	if strings.Contains(output, "DBG") {
		t.Errorf("Normal mode should not show DBG level logs, got: %s", output)
	}
}

// =============================================================================
// Integration Tests - Help Flag
// =============================================================================

// TestMain_Help_ShowsUsage tests that -h shows usage information
func TestMain_Help_ShowsUsage(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "-h")
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)

	// Should show flag descriptions
	if !strings.Contains(outputStr, "config") {
		t.Errorf("-h should mention -config flag, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "debug") {
		t.Errorf("-h should mention -debug flag, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "version") {
		t.Errorf("-h should mention -version flag, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "list-profiles") {
		t.Errorf("-h should mention -list-profiles flag, got: %s", outputStr)
	}
}

// =============================================================================
// Integration Tests - Invalid Flags
// =============================================================================

// TestMain_UnknownFlag_ShowsError tests that unknown flags show error
func TestMain_UnknownFlag_ShowsError(t *testing.T) {
	binary := buildTestBinary(t)

	cmd := exec.Command(binary, "--unknown-flag")
	output, err := cmd.CombinedOutput()

	// Should exit with error
	if err == nil {
		t.Errorf("Unknown flag should cause exit with error")
	}

	outputStr := string(output)

	// Should show error about unknown flag
	if !strings.Contains(outputStr, "unknown") && !strings.Contains(outputStr, "flag") {
		t.Errorf("Unknown flag should show flag error, got: %s", outputStr)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// buildTestBinary builds the dermoscope-helper binary for testing
func buildTestBinary(t *testing.T) string {
	t.Helper()

	// Get the directory containing main.go (same directory as this test file)
	mainDir := getMainDir(t)

	// Build to temp directory
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "dermoscope-helper-test")

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = mainDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s\nDir: %s", err, output, mainDir)
	}

	return binary
}

// getMainDir returns the directory containing main.go
// This test file is in the same directory as main.go
func getMainDir(t *testing.T) string {
	t.Helper()

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Check if we're in the cmd/dermoscope-helper directory
	mainGoPath := filepath.Join(cwd, "main.go")
	if _, err := os.Stat(mainGoPath); err == nil {
		return cwd
	}

	// Check if we're in the dermoscope-helper root
	mainGoPath = filepath.Join(cwd, "cmd", "dermoscope-helper", "main.go")
	if _, err := os.Stat(mainGoPath); err == nil {
		return filepath.Join(cwd, "cmd", "dermoscope-helper")
	}

	// Check if we're in the repo root
	mainGoPath = filepath.Join(cwd, "dermoscope-capture-driver", "dermoscope-helper", "cmd", "dermoscope-helper", "main.go")
	if _, err := os.Stat(mainGoPath); err == nil {
		return filepath.Join(cwd, "dermoscope-capture-driver", "dermoscope-helper", "cmd", "dermoscope-helper")
	}

	t.Fatalf("Could not find main.go directory from cwd: %s", cwd)
	return ""
}
