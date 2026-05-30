package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/iPmart/iPShadowT/internal/logger"
)

// Updater handles automatic updates from GitHub releases
type Updater struct {
	currentVersion string
	repoOwner      string
	repoName       string
	log            *logger.Logger
}

// Release represents a GitHub release
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset (binary file)
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// NewUpdater creates a new updater
func NewUpdater(currentVersion string, log *logger.Logger) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		repoOwner:      "iPmart",
		repoName:       "iPShadowT",
		log:            log,
	}
}

// CheckUpdate checks if a new version is available
func (u *Updater) CheckUpdate() (*Release, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.repoOwner, u.repoName)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("failed to parse release: %w", err)
	}

	if release.TagName != u.currentVersion && release.TagName > u.currentVersion {
		return &release, true, nil
	}

	return &release, false, nil
}

// Update downloads and replaces the current binary
func (u *Updater) Update(release *Release) error {
	// Find the right asset for this platform
	assetName := fmt.Sprintf("ipshadowt-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.DownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	u.log.Info("Downloading %s...", assetName)

	// Download
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "ipshadowt-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download write failed: %w", err)
	}
	tmpFile.Close()

	// Make executable
	os.Chmod(tmpFile.Name(), 0755)

	// Replace current binary
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Rename current to .bak
	backupPath := execPath + ".bak"
	os.Remove(backupPath)
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary
	if err := os.Rename(tmpFile.Name(), execPath); err != nil {
		// Restore backup
		os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	u.log.Info("✅ Updated to %s (restart required)", release.TagName)
	return nil
}
