package managed

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeyserDownloadAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	geyserURL := "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone"

	// Test HEAD request to check if we can get metadata without downloading
	t.Run("HEAD request for metadata", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, "HEAD", geyserURL, nil)
		if err != nil {
			t.Fatalf("Failed to create HEAD request: %v", err)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("HEAD request failed: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("HEAD Response Status: %s", resp.Status)
		t.Logf("Content-Length: %s", resp.Header.Get("Content-Length"))
		t.Logf("Last-Modified: %s", resp.Header.Get("Last-Modified"))
		t.Logf("ETag: %s", resp.Header.Get("ETag"))
		t.Logf("Content-Type: %s", resp.Header.Get("Content-Type"))
		t.Logf("Server: %s", resp.Header.Get("Server"))

		// Check if we get useful caching headers
		if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
			t.Logf("✅ Last-Modified header available: %s", lastModified)
		}
		if etag := resp.Header.Get("ETag"); etag != "" {
			t.Logf("✅ ETag header available: %s", etag)
		}
		if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
			t.Logf("✅ Content-Length header available: %s bytes", contentLength)
		}

		// Log all headers for analysis
		t.Logf("All headers:")
		for name, values := range resp.Header {
			for _, value := range values {
				t.Logf("  %s: %s", name, value)
			}
		}
	})

	// Test conditional GET with If-Modified-Since
	t.Run("conditional GET with If-Modified-Since", func(t *testing.T) {
		// First, get the Last-Modified header
		req, err := http.NewRequestWithContext(ctx, "HEAD", geyserURL, nil)
		if err != nil {
			t.Fatalf("Failed to create HEAD request: %v", err)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("HEAD request failed: %v", err)
		}
		resp.Body.Close()

		lastModified := resp.Header.Get("Last-Modified")
		if lastModified == "" {
			t.Skip("No Last-Modified header, skipping conditional GET test")
		}

		// Now try a conditional GET with the same timestamp
		req2, err := http.NewRequestWithContext(ctx, "GET", geyserURL, nil)
		if err != nil {
			t.Fatalf("Failed to create GET request: %v", err)
		}
		req2.Header.Set("If-Modified-Since", lastModified)

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Conditional GET request failed: %v", err)
		}
		defer resp2.Body.Close()

		t.Logf("Conditional GET Response Status: %s", resp2.Status)

		if resp2.StatusCode == 304 {
			t.Logf("✅ Server supports conditional requests (304 Not Modified)")
		} else if resp2.StatusCode == 200 {
			t.Logf("⚠️ Server returned 200 OK even with If-Modified-Since (may not support conditional requests)")
		} else {
			t.Logf("❓ Unexpected status code: %d", resp2.StatusCode)
		}
	})

	// Test conditional GET with If-None-Match (ETag)
	t.Run("conditional GET with If-None-Match", func(t *testing.T) {
		// First, get the ETag header
		req, err := http.NewRequestWithContext(ctx, "HEAD", geyserURL, nil)
		if err != nil {
			t.Fatalf("Failed to create HEAD request: %v", err)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("HEAD request failed: %v", err)
		}
		resp.Body.Close()

		etag := resp.Header.Get("ETag")
		if etag == "" {
			t.Skip("No ETag header, skipping conditional GET test")
		}

		// Now try a conditional GET with the same ETag
		req2, err := http.NewRequestWithContext(ctx, "GET", geyserURL, nil)
		if err != nil {
			t.Fatalf("Failed to create GET request: %v", err)
		}
		req2.Header.Set("If-None-Match", etag)

		resp2, err := client.Do(req2)
		if err != nil {
			t.Fatalf("Conditional GET request failed: %v", err)
		}
		defer resp2.Body.Close()

		t.Logf("Conditional GET with ETag Response Status: %s", resp2.Status)

		switch resp2.StatusCode {
		case 304:
			t.Logf("✅ Server supports ETag-based conditional requests (304 Not Modified)")
		case 200:
			t.Logf("⚠️ Server returned 200 OK even with If-None-Match (may not support ETag validation)")
		default:
			t.Logf("❓ Unexpected status code: %d", resp2.StatusCode)
		}
	})
}

func TestLocalFileModificationTime(t *testing.T) {
	// Test our ability to check local file modification times
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-geyser.jar")

	// Create a test file
	if err := os.WriteFile(testFile, []byte("fake jar content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get file info
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	modTime := info.ModTime()
	t.Logf("File modification time: %s", modTime.Format(time.RFC3339))
	t.Logf("File modification time (Unix): %d", modTime.Unix())
	t.Logf("File modification time (HTTP format): %s", modTime.Format(time.RFC1123))

	// Test time comparison
	now := time.Now()
	if modTime.After(now) {
		t.Error("File modification time should not be in the future")
	}

	// Test if file is "old" (more than 1 hour ago)
	oneHourAgo := now.Add(-1 * time.Hour)
	if modTime.Before(oneHourAgo) {
		t.Logf("File is older than 1 hour")
	} else {
		t.Logf("File is newer than 1 hour")
	}
}

func TestDownloadIfNewer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	geyserURL := "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone"

	t.Run("download new file", func(t *testing.T) {
		// Each subtest gets its own temp directory and file to avoid conflicts on Windows
		tempDir := t.TempDir()
		testJarPath := filepath.Join(tempDir, "test-geyser.jar")
		// File doesn't exist, should download
		updated, err := downloadIfNewer(ctx, geyserURL, testJarPath)
		if err != nil {
			t.Fatalf("downloadIfNewer failed: %v", err)
		}

		if !updated {
			t.Error("Expected file to be downloaded when missing")
		}

		if !fileExists(testJarPath) {
			t.Error("File should exist after download")
		}

		// Check that ETag was stored
		etagPath := testJarPath + ".etag"
		if !fileExists(etagPath) {
			t.Error("ETag file should be created")
		}

		info, err := os.Stat(testJarPath)
		if err != nil {
			t.Fatalf("Failed to stat downloaded file: %v", err)
		}

		t.Logf("Downloaded file size: %d bytes", info.Size())
		t.Logf("Downloaded file mod time: %s", info.ModTime().Format(time.RFC3339))
	})

	t.Run("check for updates when file exists", func(t *testing.T) {
		// Create our own test file to avoid Windows file conflicts
		tempDir := t.TempDir()
		testJarPath := filepath.Join(tempDir, "test-geyser.jar")

		// First download the file
		_, err := downloadIfNewer(ctx, geyserURL, testJarPath)
		if err != nil {
			t.Fatalf("Initial download failed: %v", err)
		}

		// File exists now, should check for updates
		originalInfo, err := os.Stat(testJarPath)
		if err != nil {
			t.Fatalf("Failed to stat existing file: %v", err)
		}

		updated, err := downloadIfNewer(ctx, geyserURL, testJarPath)
		if err != nil {
			t.Fatalf("downloadIfNewer failed: %v", err)
		}

		t.Logf("File was updated: %v", updated)

		// Verify file still exists
		if !fileExists(testJarPath) {
			t.Error("File should still exist after update check")
		}

		newInfo, err := os.Stat(testJarPath)
		if err != nil {
			t.Fatalf("Failed to stat file after update check: %v", err)
		}

		if updated {
			t.Logf("File was updated - old size: %d, new size: %d", originalInfo.Size(), newInfo.Size())
		} else {
			t.Logf("File was not updated (already up to date)")
			// File should be unchanged
			if originalInfo.Size() != newInfo.Size() {
				t.Error("File size should not change when not updated")
			}
		}
	})

	t.Run("immediate second check should not update", func(t *testing.T) {
		// Create our own test file to avoid Windows file conflicts
		tempDir := t.TempDir()
		testJarPath := filepath.Join(tempDir, "test-geyser.jar")

		// First download the file
		_, err := downloadIfNewer(ctx, geyserURL, testJarPath)
		if err != nil {
			t.Fatalf("Initial download failed: %v", err)
		}

		// Immediately check again - should definitely not update
		updated, err := downloadIfNewer(ctx, geyserURL, testJarPath)
		if err != nil {
			t.Fatalf("downloadIfNewer failed: %v", err)
		}

		if updated {
			t.Error("File should not be updated on immediate second check")
		}

		t.Logf("✅ Immediate second check correctly returned not updated")
	})
}

func TestETagStorage(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.jar")
	testETag := `"0e1a44644b1cb7159bf4065a92efcfa4d"`

	// Create test file
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test storing ETag
	storeETag(testFile, testETag)

	// Test retrieving ETag
	retrievedETag := getStoredETag(testFile)
	if retrievedETag != testETag {
		t.Errorf("ETag mismatch: got %q, want %q", retrievedETag, testETag)
	}

	// Verify ETag file exists
	etagPath := testFile + ".etag"
	if !fileExists(etagPath) {
		t.Error("ETag file should exist")
	}

	// Test retrieving non-existent ETag
	nonExistentFile := filepath.Join(tempDir, "nonexistent.jar")
	emptyETag := getStoredETag(nonExistentFile)
	if emptyETag != "" {
		t.Errorf("Expected empty ETag for non-existent file, got %q", emptyETag)
	}
}

func TestGeyserReadyDetection(t *testing.T) {
	tests := []struct {
		name        string
		logLines    []string
		expectReady bool
		description string
	}{
		{
			name: "geyser startup sequence",
			logLines: []string{
				"[10:51:12 INFO] Using config file /path/to/config.yml",
				"[10:51:12 INFO] Loading extensions...",
				"[10:51:12 INFO] Loaded 0 extension(s)",
				"[10:51:13 INFO] ******************************************",
				"[10:51:13 INFO] Loading Geyser version 2.8.3-b917 (git-master-1dc3f41)",
				"[10:51:13 INFO] ******************************************",
				"[10:51:16 INFO] Started Geyser on UDP port 19132",
				"[10:51:16 INFO] Done (2.336s)! Run /geyser help for help!",
			},
			expectReady: true,
			description: "Should detect ready when Done message appears",
		},
		{
			name: "incomplete startup",
			logLines: []string{
				"[10:51:12 INFO] Using config file /path/to/config.yml",
				"[10:51:12 INFO] Loading extensions...",
				"[10:51:13 INFO] Loading Geyser version 2.8.3-b917 (git-master-1dc3f41)",
			},
			expectReady: false,
			description: "Should not detect ready without Done message",
		},
		{
			name: "different done message format",
			logLines: []string{
				"[10:51:16 INFO] Started Geyser on UDP port 19132",
				"[10:51:16 INFO] Done (1.234s)! Run /geyser help for help!",
			},
			expectReady: true,
			description: "Should detect ready with different timing",
		},
		{
			name: "false positive",
			logLines: []string{
				"[10:51:16 INFO] Loading is done",
				"[10:51:16 INFO] Run some command",
				"[10:51:16 INFO] Help is available",
			},
			expectReady: false,
			description: "Should not trigger on partial matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a pipe to simulate Geyser output
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			defer r.Close()
			defer w.Close()

			// Create a buffer to capture the prefixed output
			var outputBuffer strings.Builder
			readyCh := make(chan struct{})

			// Create a runner (we don't need a real config for this test)
			runner := &Runner{}

			// Start handleOutput in a goroutine
			go runner.handleOutput(r, "[GEYSER] ", &outputBuffer, readyCh)

			// Write test log lines
			go func() {
				defer w.Close()
				for _, line := range tt.logLines {
					fmt.Fprintf(w, "%s\n", line)
					time.Sleep(10 * time.Millisecond) // Small delay to simulate real output
				}
			}()

			// Wait for ready signal or timeout
			timeout := time.After(1 * time.Second)
			var gotReady bool

			select {
			case <-readyCh:
				gotReady = true
			case <-timeout:
				gotReady = false
			}

			if gotReady != tt.expectReady {
				t.Errorf("Ready detection: got %v, want %v. %s", gotReady, tt.expectReady, tt.description)
			}

			// Verify output was properly prefixed
			output := outputBuffer.String()
			for _, originalLine := range tt.logLines {
				expectedLine := "[GEYSER] " + originalLine
				if !strings.Contains(output, expectedLine) {
					t.Errorf("Expected output to contain %q, but got:\n%s", expectedLine, output)
				}
			}

			t.Logf("Output:\n%s", output)
		})
	}
}

func TestShouldUpdateLogic(t *testing.T) {
	tests := []struct {
		name           string
		autoUpdate     bool
		fileExists     bool
		fileAge        time.Duration
		serverModified string
		expectedUpdate bool
		description    string
	}{
		{
			name:           "autoUpdate disabled, file exists",
			autoUpdate:     false,
			fileExists:     true,
			expectedUpdate: false,
			description:    "Should not update when autoUpdate is disabled and file exists",
		},
		{
			name:           "autoUpdate disabled, file missing",
			autoUpdate:     false,
			fileExists:     false,
			expectedUpdate: true,
			description:    "Should download when file is missing, even with autoUpdate disabled",
		},
		{
			name:           "autoUpdate enabled, file missing",
			autoUpdate:     true,
			fileExists:     false,
			expectedUpdate: true,
			description:    "Should download when file is missing",
		},
		{
			name:           "autoUpdate enabled, file exists, no server info",
			autoUpdate:     true,
			fileExists:     true,
			expectedUpdate: true,
			description:    "Should update when autoUpdate enabled and no server info available (current behavior)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the current logic from the code
			shouldUpdate := tt.autoUpdate || !tt.fileExists

			if shouldUpdate != tt.expectedUpdate {
				t.Errorf("shouldUpdate: got %v, want %v. %s", shouldUpdate, tt.expectedUpdate, tt.description)
			}
		})
	}
}
