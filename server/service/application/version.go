package application

import (
	"NanoKVM-Server/proto"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type Latest struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Sha512  string `json:"sha512"`
	Size    uint   `json:"size"`
	Url     string `json:"url"`
}

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

	choice := resolveUpdate(currentVersion)

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

func resolveUpdate(current string) updateChoice {
	official, fork := fetchCandidates()
	forkVersion := ""
	if fork != nil {
		forkVersion = fork.Version
	}

	latest, source := selectUpdate(current, official, forkVersion)
	choice := updateChoice{Version: latest, Source: source}
	if source == sourceFork {
		choice.Fork = fork
	}

	return choice
}

func fetchCandidates() (string, *ForkRelease) {
	var (
		wg       sync.WaitGroup
		official string
		fork     *ForkRelease
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		latest, err := getLatest()
		if err != nil {
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

	return official, fork
}

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
	baseURL := StableURL
	if isPreviewEnabled() {
		baseURL = PreviewURL
	}

	url := fmt.Sprintf("%s/nanokvm_pro_latest.json?now=%d", baseURL, time.Now().Unix())
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Errorf("failed to get latest version: %v", err)
		return nil, err
	}
	req.Header.Set("User-Agent", updateUserAgent)

	resp, err := updateHTTP.Do(req)
	if err != nil {
		log.Errorf("failed to get latest version: %v", err)
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("failed to read response: %v", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Errorf("server responded with status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	var latest Latest
	if err := json.Unmarshal(body, &latest); err != nil {
		log.Errorf("failed to unmarshal response: %s", err)
		return nil, err
	}

	latest.Url = fmt.Sprintf("%s/%s", baseURL, latest.Name)

	log.Debugf("get application latest version: %s", latest.Version)
	return &latest, nil
}
