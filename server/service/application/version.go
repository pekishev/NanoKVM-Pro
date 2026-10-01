package application

import (
	"NanoKVM-Server/proto"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Latest is the bounded manifest data needed to locate and verify an update.
type Latest struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Sha512  string `json:"sha512"`
	Size    uint64 `json:"size"`
	Url     string `json:"url"`
}

const maxUpdateManifestBytes = 64 << 10

var latestApplicationNamePattern = regexp.MustCompile(`^nanokvm_pro_([A-Za-z0-9][A-Za-z0-9._+-]{0,127})\.tar\.gz$`)

// GetVersion reports the installed version and the latest version from the
// currently selected source.
func (s *Service) GetVersion(c *gin.Context) {
	var rsp proto.Response

	currentVersion := getCurrentVersion()
	if !isUpdateCheckEnabled() {
		rsp.OkRspWithData(c, &proto.GetVersionRsp{
			Current:  currentVersion,
			Latest:   currentVersion,
			Checking: false,
		})
		log.Debugf("update check is disabled, current version: %s", currentVersion)
		return
	}

	choice, err := resolveUpdate(currentVersion)
	if err != nil {
		log.Errorf("failed to query custom update source: %v", err)
		rsp.ErrRsp(c, -1, "failed to query update source")
		return
	}

	rsp.OkRspWithData(c, &proto.GetVersionRsp{
		Current:  currentVersion,
		Latest:   choice.Version,
		Source:   choice.Source,
		Checking: true,
	})
	log.Debugf("current version: %s, latest version: %s, source: %s", currentVersion, choice.Version, choice.Source)
}

type updateChoice struct {
	Version string
	Source  string
	Fork    *ForkRelease
}

// resolveUpdate picks the newest of the selected source and the fork releases.
// It fails only when a custom source is configured and cannot be queried, so
// a broken custom source is reported instead of silently falling back.
func resolveUpdate(current string) (updateChoice, error) {
	official, fork, officialErr := fetchCandidates()
	if officialErr != nil {
		cfg, cfgErr := loadUpdateSourceConfig()
		if cfgErr != nil {
			return updateChoice{}, cfgErr
		}
		if cfg.Enabled {
			return updateChoice{}, officialErr
		}
	}

	forkVersion := ""
	if fork != nil {
		forkVersion = fork.Version
	}

	latest, source := selectUpdate(current, official, forkVersion)
	choice := updateChoice{Version: latest, Source: source}
	if source == sourceFork {
		choice.Fork = fork
	}

	return choice, nil
}

func fetchCandidates() (string, *ForkRelease, error) {
	var (
		wg          sync.WaitGroup
		official    string
		officialErr error
		fork        *ForkRelease
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		latest, err := getLatest()
		if err != nil {
			officialErr = err
			return
		}
		official = latest.Version
	}()
	go func() {
		defer wg.Done()
		release, err := getForkLatest()
		if err != nil {
			return
		}
		fork = release
	}()
	wg.Wait()

	return official, fork, officialErr
}

// getCurrentVersion reads the version exposed by the installed application.
func getCurrentVersion() string {
	defaultVersion := "v1.0.0"

	versionFile := filepath.Join(AppDir, "version")
	content, err := os.ReadFile(versionFile)
	if err != nil {
		return defaultVersion
	}

	version := strings.ReplaceAll(string(content), "\n", "")
	if version == "" {
		return defaultVersion
	}

	return version
}

func getLatest() (*Latest, error) {
	// The configured source is resolved for each check so resetting the source
	// takes effect without restarting the service.
	baseURL, err := resolveApplicationUpdateBaseURL()
	if err != nil {
		return nil, err
	}

	manifestURL := joinUpdateURL(baseURL, "nanokvm_pro_latest.json") + "?now=" + strconv.FormatInt(time.Now().Unix(), 10)
	body, err := readUpdateManifest(manifestURL, maxUpdateManifestBytes)
	if err != nil {
		log.Errorf("failed to read latest version manifest: %v", err)
		return nil, err
	}

	var latest Latest
	if err := json.Unmarshal(body, &latest); err != nil {
		log.Errorf("failed to unmarshal response: %s", err)
		return nil, err
	}
	if err := validateLatest(&latest); err != nil {
		return nil, err
	}

	latest.Url = joinUpdateURL(baseURL, latest.Name)

	log.Debugf("get application latest version: %s", latest.Version)
	return &latest, nil
}

func validateLatest(latest *Latest) error {
	// Validate names and hashes before constructing a download URL from a
	// manifest supplied by either the official or a custom source.
	if latest == nil || !validArtifactVersion(latest.Version) {
		return errors.New("invalid application update version")
	}
	matches := latestApplicationNamePattern.FindStringSubmatch(latest.Name)
	if len(matches) != 2 || matches[1] != latest.Version {
		return errors.New("invalid application update package name")
	}
	if err := validateSHA512String(latest.Sha512); err != nil {
		return errors.New("invalid application update checksum")
	}
	return nil
}
