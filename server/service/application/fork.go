package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const forkReleasesTimeout = 20 * time.Second

type ForkRelease struct {
	Version string
	Name    string
	URL     string
	SHA256  string
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Draft   bool      `json:"draft"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func getForkLatest() (*ForkRelease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), forkReleasesTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forkReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", updateUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		log.Errorf("failed to get fork releases: %v", err)
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		log.Errorf("failed to read fork releases: %v", err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Errorf("fork releases responded with status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		log.Errorf("failed to unmarshal fork releases: %s", err)
		return nil, err
	}

	latest := newestForkRelease(releases)
	if latest == nil {
		return nil, fmt.Errorf("fork release archive not found")
	}

	log.Debugf("get fork latest version: %s", latest.Version)
	return latest, nil
}

func newestForkRelease(releases []ghRelease) *ForkRelease {
	var best *ForkRelease

	for _, release := range releases {
		if release.Draft {
			continue
		}

		version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if !parseAppVersion(version).ok {
			continue
		}

		name, downloadURL, sha256, ok := forkArchive(release)
		if !ok {
			continue
		}

		candidate := &ForkRelease{
			Version: version,
			Name:    name,
			URL:     downloadURL,
			SHA256:  sha256,
		}
		if best == nil || versionNewer(candidate.Version, best.Version) {
			best = candidate
		}
	}

	return best
}

func forkArchive(release ghRelease) (string, string, string, bool) {
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	wanted := "nanokvm-pro-" + version + ".tar.gz"
	var fallback *ghAsset

	for i := range release.Assets {
		asset := &release.Assets[i]
		if !strings.HasPrefix(asset.Name, "nanokvm-pro-") || !strings.HasSuffix(asset.Name, ".tar.gz") {
			continue
		}
		if asset.Name == wanted {
			return asset.Name, asset.BrowserDownloadURL, digestSHA256(asset.Digest), true
		}
		if fallback == nil {
			fallback = asset
		}
	}

	if fallback == nil {
		return "", "", "", false
	}

	return fallback.Name, fallback.BrowserDownloadURL, digestSHA256(fallback.Digest), true
}

func digestSHA256(digest string) string {
	digest = strings.ToLower(strings.TrimSpace(digest))
	return strings.TrimPrefix(digest, "sha256:")
}

func installFork(rel *ForkRelease) error {
	if rel == nil || rel.URL == "" {
		return fmt.Errorf("fork release is empty")
	}

	workspace, err := newOnlineUpdateWorkspace()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	_ = sendMessage("download", 0)
	archivePath := filepath.Join(workspace, "release.tar.gz")
	if err := download(rel.URL, archivePath); err != nil {
		log.Errorf("download fork release failed: %s", err)
		return err
	}
	if rel.SHA256 != "" {
		if err := checksumSHA256(archivePath, rel.SHA256); err != nil {
			log.Errorf("check fork sha256 failed: %s", err)
			return err
		}
	}

	extractDir := filepath.Join(workspace, "release")
	if err := extractRelease(archivePath, extractDir); err != nil {
		log.Errorf("failed to extract fork release: %s", err)
		return err
	}
	if _, err := installRelease(extractDir); err != nil {
		log.Errorf("failed to install fork release: %s", err)
		return err
	}

	_ = sendMessage("restart", 0)
	return nil
}
