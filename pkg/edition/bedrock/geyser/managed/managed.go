package managed

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/util/netutil"

	"go.minekube.com/gate/pkg/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/yaml.v3"
)

// Runner manages a managed Geyser Standalone process.
type Runner struct {
	cfg    *bconfig.Config
	cmd    *exec.Cmd
	mu     sync.Mutex
	cancel context.CancelFunc
}

func New(cfg *bconfig.Config) *Runner { return &Runner{cfg: cfg} }

// EnsureKey generates the Floodgate key if it doesn't exist.
// This is called early during integration setup to ensure the key exists before validation.
func (r *Runner) EnsureKey(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("managed")
	return r.ensureFloodgateKey(ctx, log)
}

// Ensure downloads the Geyser jar if missing, optionally updating it.
// Also ensures the Floodgate key exists, generating one if needed.
func (r *Runner) Ensure(ctx context.Context) (string, error) {
	managed := r.cfg.GetManaged()
	dataDir := managed.DataDir
	if dataDir == "" {
		dataDir = ".geyser"
	}

	log := logr.FromContextOrDiscard(ctx).WithName("geyser.managed")
	log.Info("ensuring geyser jar", "dataDir", dataDir, "autoUpdate", managed.AutoUpdate)

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("creating managed dir: %w", err)
	}

	// Ensure Floodgate key exists
	if err := r.ensureFloodgateKey(ctx, log); err != nil {
		return "", fmt.Errorf("failed to ensure floodgate key: %w", err)
	}

	jarPath := filepath.Join(dataDir, "geyser-standalone.jar")

	if !fileExists(jarPath) {
		log.Info("downloading geyser standalone (missing)", "url", managed.JarURL, "path", jarPath)
		if err := download(ctx, managed.JarURL, jarPath); err != nil {
			return "", fmt.Errorf("failed to download geyser: %w", err)
		}
		log.Info("geyser jar downloaded successfully", "path", jarPath)
	} else if managed.AutoUpdate {
		log.Info("checking for geyser updates", "url", managed.JarURL, "path", jarPath)
		updated, err := downloadIfNewer(ctx, managed.JarURL, jarPath)
		if err != nil {
			return "", fmt.Errorf("failed to check/download geyser updates: %w", err)
		}
		if updated {
			log.Info("geyser jar updated successfully", "path", jarPath)
		} else {
			log.Info("geyser jar is up to date", "path", jarPath)
		}
	} else {
		log.Info("using existing geyser jar (autoUpdate disabled)", "path", jarPath)
	}
	return jarPath, nil
}

// Start runs the Geyser process with provided config.
func (r *Runner) Start(ctx context.Context, jarPath string, extraArgs ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil {
		return fmt.Errorf("geyser already running")
	}

	log := logr.FromContextOrDiscard(ctx).WithName("managed")
	managed := r.cfg.GetManaged()

	// Create Geyser config file in the data directory
	configPath, err := r.writeGeyserConfig(managed)
	if err != nil {
		return fmt.Errorf("failed to write geyser config: %w", err)
	}

	// Convert to absolute paths to avoid working directory issues
	absJarPath, err := filepath.Abs(jarPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute jar path: %w", err)
	}
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute config path: %w", err)
	}

	// Build command args - Geyser uses config file, not CLI args for most settings
	args := []string{"-jar", absJarPath, "--nogui", "--config", absConfigPath}
	args = append(args, managed.ExtraArgs...)
	args = append(args, extraArgs...)

	log.Info("starting geyser standalone process",
		"java", managed.JavaPath,
		"jar", absJarPath,
		"config", absConfigPath,
		"bedrockPort", getBedrockPort(managed.ConfigOverrides),
		"args", args)

	cmd := exec.CommandContext(ctx, managed.JavaPath, args...)
	cmd.Dir = filepath.Dir(absJarPath) // Run in the jar directory

	// Create pipes to capture output and wait for ready signal
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start geyser process: %w", err)
	}

	r.cmd = cmd
	log.Info("geyser standalone process started", "pid", cmd.Process.Pid)

	// Start goroutines to handle output and wait for ready signal
	readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer readyCancel()

	readyCh := make(chan struct{})

	// Handle stdout with ready detection
	go r.handleOutput(stdout, "[GEYSER] ", os.Stdout, readyCh)
	// Handle stderr (no ready detection needed)
	go r.handleOutput(stderr, "[GEYSER] ", os.Stderr, nil)

	// Wait for Geyser to be ready or timeout
	select {
	case <-readyCh:
		log.Info("geyser is ready and accepting connections")
		return nil
	case <-readyCtx.Done():
		log.Info("timeout waiting for geyser to be ready, continuing anyway")
		return nil // Don't fail, just continue
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleOutput reads from a pipe, writes to destination with prefix, and optionally signals when ready
func (r *Runner) handleOutput(pipe io.ReadCloser, prefix string, dest io.Writer, readyCh chan<- struct{}) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)
	readySignaled := false

	for scanner.Scan() {
		line := scanner.Text()

		// Write the prefixed line to destination
		fmt.Fprintf(dest, "%s%s\n", prefix, line)

		// Check for Geyser ready signal
		if readyCh != nil && !readySignaled {
			// Look for the "Done" message that indicates Geyser is fully started
			if strings.Contains(line, "Done (") && strings.Contains(line, ")! Run /geyser help for help!") {
				close(readyCh)
				readySignaled = true
			}
		}
	}
}

// Stop stops the process if running.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil {
		return
	}

	log := logr.FromContextOrDiscard(context.Background()).WithName("managed")
	log.Info("stopping geyser standalone process", "pid", r.cmd.Process.Pid)

	_ = r.cmd.Process.Kill()
	_ = r.cmd.Wait()
	r.cmd = nil

	log.Info("geyser standalone process stopped")
}

// writeGeyserConfig creates a Geyser config file based on Gate's settings.
func (r *Runner) writeGeyserConfig(managed bconfig.ManagedGeyser) (string, error) {
	dataDir := managed.DataDir
	if dataDir == "" {
		dataDir = ".geyser"
	}

	configPath := filepath.Join(dataDir, "config.yml")

	// Generate absolute path for Floodgate key
	absKeyPath, err := filepath.Abs(r.cfg.FloodgateKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute key path: %w", err)
	}

	// Generate Geyser config based on our existing example
	// Use Gate's Geyser listen address as the remote address
	// Default bedrock port is 19132 - can be overridden via configOverrides.bedrock.port
	geyserConfig := fmt.Sprintf(`# Auto-generated Geyser config by Gate managed mode
bedrock:
  port: 19132
  motd1: "Gate + Geyser"
  motd2: "Managed Cross-Play"
  server-name: "Gate Bedrock"
  compression-level: 6

remote:
  # Connect to Gate's Bedrock listener
  address: %s
  port: %d
  auth-type: floodgate
  use-proxy-protocol: true
  forward-hostname: false

# Point to the shared Floodgate key (absolute path)
floodgate-key-file: %s

# Enable passthrough for better integration
passthrough-motd: true
passthrough-player-counts: true
legacy-ping-passthrough: false
ping-passthrough-interval: 3

# Performance settings
forward-player-ping: true
max-players: 100
debug-mode: false

# Bedrock-specific settings
show-cooldown: title
show-coordinates: true
disable-bedrock-scaffolding: false
emote-offhand-workaround: "disabled"

# Custom skulls and items
allow-custom-skulls: true
max-visible-custom-skulls: 128
custom-skull-render-distance: 32
add-non-bedrock-items: true

# Resource packs
force-resource-packs: true

# Xbox features
xbox-achievements-enabled: false

# Logging
log-player-ip-addresses: true
notify-on-new-bedrock-update: true

# Advanced settings
scoreboard-packet-threshold: 20
enable-proxy-connections: false
mtu: 1400
use-direct-connection: true
disable-compression: true

config-version: 4
`,
		extractHost(r.cfg.GeyserListenAddr),
		extractPort(r.cfg.GeyserListenAddr),
		absKeyPath)

	// Apply user config overrides if specified
	if len(managed.ConfigOverrides) > 0 {
		geyserConfig, err = r.applyConfigOverrides(geyserConfig, managed.ConfigOverrides)
		if err != nil {
			return "", fmt.Errorf("failed to apply config overrides: %w", err)
		}
	}

	if err := os.WriteFile(configPath, []byte(geyserConfig), 0o644); err != nil {
		return "", fmt.Errorf("failed to write geyser config: %w", err)
	}

	return configPath, nil
}

// extractHost extracts the host from "host:port" format
func extractHost(addr string) string {
	return netutil.HostStr(addr)
}

// extractPort extracts the port from "host:port" format
func extractPort(addr string) int {
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		if port, err := strconv.Atoi(addr[idx+1:]); err == nil {
			return port
		}
	}
	return 25567 // Default
}

// applyConfigOverrides applies user-specified config overrides to the generated Geyser config
func (r *Runner) applyConfigOverrides(baseConfig string, overrides map[string]any) (string, error) {
	// Parse the base config as YAML
	var configMap map[string]any
	if err := yaml.Unmarshal([]byte(baseConfig), &configMap); err != nil {
		return "", fmt.Errorf("failed to parse base config: %w", err)
	}

	// Apply overrides using deep merge
	mergeConfigMaps(configMap, overrides)

	// Convert back to YAML
	result, err := yaml.Marshal(configMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated config: %w", err)
	}

	return string(result), nil
}

// mergeConfigMaps recursively merges override values into the base map
func mergeConfigMaps(base, override map[string]any) {
	for key, value := range override {
		if baseValue, exists := base[key]; exists {
			// If both are maps, merge recursively
			if baseMap, ok := baseValue.(map[string]any); ok {
				if overrideMap, ok := value.(map[string]any); ok {
					mergeConfigMaps(baseMap, overrideMap)
					continue
				}
			}
		}
		// Otherwise, override the value
		base[key] = value
	}
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// getBedrockPort extracts the bedrock port from config overrides, defaulting to 19132
func getBedrockPort(configOverrides map[string]any) int {
	const defaultBedrockPort = 19132

	if configOverrides == nil {
		return defaultBedrockPort
	}

	// Check for bedrock.port in config overrides
	if bedrockConfig, ok := configOverrides["bedrock"]; ok {
		if bedrockMap, ok := bedrockConfig.(map[string]any); ok {
			if port, ok := bedrockMap["port"]; ok {
				if portInt, ok := port.(int); ok {
					return portInt
				}
			}
		}
	}

	return defaultBedrockPort
}

func download(ctx context.Context, url, dest string) error {
	// Add timeout for download
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// Use proper HTTP client with User-Agent and instrumentation
	client := &http.Client{
		Timeout:   10 * time.Minute,
		Transport: otelhttp.NewTransport(withHeader(http.DefaultTransport, version.UserAgentHeader())),
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	// Write the file content
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()      // Ensure file is closed on error
		os.Remove(tmp) // Clean up temp file
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()      // Ensure file is closed on error
		os.Remove(tmp) // Clean up temp file
		return err
	}

	// Explicitly close file before rename (critical for Windows compatibility)
	if err := f.Close(); err != nil {
		os.Remove(tmp) // Clean up temp file
		return err
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp) // Clean up temp file if rename fails
		return err
	}
	// Set file modification time to server's Last-Modified if available
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if serverTime, err := time.Parse(time.RFC1123, lastModified); err == nil {
			os.Chtimes(dest, serverTime, serverTime)
		}
	}

	// Store ETag for future requests
	if etag := resp.Header.Get("ETag"); etag != "" {
		storeETag(dest, etag)
	}

	return nil
}

// downloadIfNewer checks if a newer version is available and downloads it if so.
// Returns true if the file was updated, false if it was already up to date.
func downloadIfNewer(ctx context.Context, url, path string) (bool, error) {
	// Get local file info
	fileInfo, err := os.Stat(path)
	if err != nil {
		// File doesn't exist, download it
		return true, download(ctx, url, path)
	}

	client := &http.Client{
		Transport: otelhttp.NewTransport(withHeader(http.DefaultTransport, version.UserAgentHeader())),
		Timeout:   30 * time.Second, // Shorter timeout for HEAD request
	}

	// Create conditional GET request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	// Use If-Modified-Since based on local file time
	req.Header.Set("If-Modified-Since", fileInfo.ModTime().Format(time.RFC1123))

	// Also try ETag if we have one stored
	if etag := getStoredETag(path); etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	client.Timeout = 5 * time.Minute // Longer timeout for potential download
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("conditional GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		// File hasn't changed
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download failed: %s", resp.Status)
	}

	// Download the new version
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return false, fmt.Errorf("creating temp file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return false, fmt.Errorf("writing file: %w", err)
	}

	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("syncing file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return false, fmt.Errorf("moving temp file: %w", err)
	}

	// Set file modification time to server's Last-Modified if available
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		if serverTime, err := time.Parse(time.RFC1123, lastModified); err == nil {
			os.Chtimes(path, serverTime, serverTime)
		}
	}

	// Store ETag for future requests
	if etag := resp.Header.Get("ETag"); etag != "" {
		storeETag(path, etag)
	}

	return true, nil
}

// getStoredETag retrieves a stored ETag for a file
func getStoredETag(path string) string {
	// Store ETag in a sidecar file
	etagPath := path + ".etag"
	data, err := os.ReadFile(etagPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// storeETag stores an ETag for a file
func storeETag(path, etag string) {
	// Store ETag in a sidecar file
	etagPath := path + ".etag"
	os.WriteFile(etagPath, []byte(etag), 0644)
}

func withHeader(rt http.RoundTripper, header http.Header) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return headerRoundTripper{Header: header, rt: rt}
}

type headerRoundTripper struct {
	http.Header
	rt http.RoundTripper
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Header {
		req.Header[k] = v
	}
	return h.rt.RoundTrip(req)
}

// ensureFloodgateKey generates a Floodgate key if it doesn't exist.
func (r *Runner) ensureFloodgateKey(ctx context.Context, log logr.Logger) error {
	keyPath := r.cfg.FloodgateKeyPath
	if keyPath == "" {
		return fmt.Errorf("floodgate key path not configured")
	}

	// Check if key already exists
	if fileExists(keyPath) {
		log.V(1).Info("floodgate key already exists", "path", keyPath)
		return nil
	}

	log.Info("generating floodgate key", "path", keyPath)

	// Use the Floodgate package's key generation
	if err := floodgate.GenerateKeyToFile(keyPath); err != nil {
		return fmt.Errorf("failed to generate floodgate key: %w", err)
	}

	log.Info("floodgate key generated successfully", "path", keyPath)
	return nil
}
