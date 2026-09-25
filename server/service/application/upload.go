package application

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/proto"
)

const (
	maxReleaseBytes = 256 << 20
	maxExtractBytes = 512 << 20

	serverBinary = "/kvmapp/server/NanoKVM-Server"
	webDir       = "/kvmapp/server/web"
	versionFile  = "/kvmapp/version"
)

func (s *Service) Upload(c *gin.Context) {
	var rsp proto.Response

	updateMutex.Lock()
	if isUpdating {
		updateMutex.Unlock()
		rsp.ErrRsp(c, -1, "update already in progress")
		return
	}
	isUpdating = true
	updateMutex.Unlock()

	defer func() {
		updateMutex.Lock()
		isUpdating = false
		updateMutex.Unlock()
	}()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReleaseBytes)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		rsp.ErrRsp(c, -2, "bad request")
		return
	}
	defer func() {
		_ = file.Close()
	}()

	if !isReleaseArchive(header.Filename) {
		rsp.ErrRsp(c, -3, "expected a .tar.gz release archive")
		return
	}

	if err := os.RemoveAll(TempDir); err != nil {
		log.Errorf("failed to clean %s: %v", TempDir, err)
		rsp.ErrRsp(c, -4, "failed to prepare update")
		return
	}
	if err := os.MkdirAll(TempDir, 0o755); err != nil {
		log.Errorf("failed to create %s: %v", TempDir, err)
		rsp.ErrRsp(c, -4, "failed to prepare update")
		return
	}

	archivePath := filepath.Join(TempDir, "release.tar.gz")
	if err := saveUpload(file, archivePath); err != nil {
		log.Errorf("failed to save release: %v", err)
		rsp.ErrRsp(c, -5, "failed to save archive")
		return
	}

	extractDir := filepath.Join(TempDir, "release")
	if err := extractRelease(archivePath, extractDir); err != nil {
		log.Errorf("failed to extract release: %v", err)
		rsp.ErrRsp(c, -6, "invalid release archive")
		return
	}

	version, err := installRelease(extractDir)
	if err != nil {
		log.Errorf("failed to install release: %v", err)
		rsp.ErrRsp(c, -7, "failed to install release")
		return
	}

	_ = sendMessage("install", 100)
	_ = sendMessage("restart", 0)

	rsp.OkRspWithData(c, &proto.UploadReleaseRsp{Version: version})
	log.Infof("installed release %s", version)

	go restartService()
}

func isReleaseArchive(name string) bool {
	lower := strings.ToLower(filepath.Base(name))
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

func saveUpload(src io.Reader, target string) error {
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = dst.Close()
	}()

	_, err = io.Copy(dst, io.LimitReader(src, maxReleaseBytes+1))
	return err
}

func extractRelease(archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = gzipReader.Close()
	}()

	tarReader := tar.NewReader(gzipReader)
	var extracted int64

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := filepath.Clean(header.Name)
		if !allowedReleasePath(name) {
			return fmt.Errorf("unexpected file %s", header.Name)
		}

		target, err := safeJoin(destDir, name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			written, err := writeExtractedFile(target, tarReader, header.Size, maxExtractBytes-extracted)
			if err != nil {
				return err
			}
			extracted += written
		default:
			return fmt.Errorf("unsupported entry %s", header.Name)
		}
	}

	if _, err := os.Stat(filepath.Join(destDir, "NanoKVM-Server")); err != nil {
		return fmt.Errorf("archive has no NanoKVM-Server")
	}
	if _, err := os.Stat(filepath.Join(destDir, "web", "index.html")); err != nil {
		return fmt.Errorf("archive has no web/index.html")
	}

	return nil
}

func allowedReleasePath(name string) bool {
	switch name {
	case "NanoKVM-Server", "version", "INSTALL.txt", "web":
		return true
	default:
		return name == "web" || strings.HasPrefix(name, "web"+string(filepath.Separator))
	}
}

func safeJoin(destDir, name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid path %s", name)
	}

	target := filepath.Join(destDir, clean)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path %s", name)
	}

	return target, nil
}

func writeExtractedFile(target string, src io.Reader, size int64, remaining int64) (int64, error) {
	if size < 0 || size > remaining {
		return 0, fmt.Errorf("release archive is too large")
	}

	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = file.Close()
	}()

	written, err := io.Copy(file, io.LimitReader(src, size))
	if err != nil {
		return written, err
	}
	if written != size {
		return written, fmt.Errorf("short read for %s", target)
	}

	return written, nil
}

func installRelease(dir string) (string, error) {
	_ = sendMessage("install", 10)

	version := readReleaseVersion(filepath.Join(dir, "version"))

	webBackup := webDir + ".bak"
	webNew := webDir + ".new"
	_ = os.RemoveAll(webNew)
	if err := copyTree(filepath.Join(dir, "web"), webNew); err != nil {
		_ = os.RemoveAll(webNew)
		return "", err
	}

	_ = os.RemoveAll(webBackup)
	if err := os.Rename(webDir, webBackup); err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(webNew)
		return "", err
	}
	if err := os.Rename(webNew, webDir); err != nil {
		_ = os.Rename(webBackup, webDir)
		_ = os.RemoveAll(webNew)
		return "", err
	}

	_ = sendMessage("install", 40)

	if err := replaceFile(filepath.Join(dir, "NanoKVM-Server"), serverBinary, 0o755); err != nil {
		_ = os.RemoveAll(webDir)
		_ = os.Rename(webBackup, webDir)
		return "", err
	}
	_ = os.RemoveAll(webBackup)

	_ = sendMessage("install", 80)

	if version != "" {
		if err := os.WriteFile(versionFile, []byte(version+"\n"), 0o644); err != nil {
			return "", err
		}
	}

	return version, nil
}

func readReleaseVersion(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(content))
}

func replaceFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmp := dst + ".new"
	if err := copyFile(src, tmp, mode); err != nil {
		return err
	}

	return os.Rename(tmp, dst)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file %s", rel)
		}

		return copyFile(path, target, 0o644)
	})
}

func restartService() {
	time.Sleep(time.Second)
	output, err := exec.Command("systemctl", "restart", "nanokvm").CombinedOutput()
	if err != nil {
		log.Errorf("failed to restart nanokvm: %s %s", err, string(output))
	}
}
