package release

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxDownload = 128 << 20

var installMutex sync.Mutex

func managedBinary() (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}
	self, err := os.Executable()
	if err != nil {
		return "", false
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil || !strings.HasSuffix(self, "/opt/playnux/playnux") {
		return "", false
	}
	marker, err := os.Lstat(filepath.Join(filepath.Dir(self), ".managed"))
	return self, err == nil && marker.Mode().IsRegular()
}

func validDigest(digest string) bool {
	hexText, ok := strings.CutPrefix(digest, "sha256:")
	decoded, err := hex.DecodeString(hexText)
	return ok && err == nil && len(decoded) == sha256.Size
}

func Install(ctx context.Context, version string) (string, error) {
	installMutex.Lock()
	defer installMutex.Unlock()
	update, err := Check(ctx)
	if err != nil {
		return "", err
	}
	binary, managed := managedBinary()
	if !managed || !update.Installable || update.Latest != version {
		return "", fmt.Errorf("this update is unavailable for automatic installation")
	}
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: checkReleaseRedirect}
	if err := installBinary(ctx, client, binary, version, update.asset); err != nil {
		return "", err
	}
	return binary, nil
}

func installBinary(ctx context.Context, client *http.Client, binary, version string, asset releaseAsset) error {
	staging, err := os.MkdirTemp(filepath.Dir(binary), ".playnux-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	archive, err := download(ctx, client, asset, staging)
	if err != nil {
		return err
	}
	defer archive.Close()
	next := filepath.Join(staging, "playnux")
	if err := unpackBinary(archive, strings.TrimSuffix(asset.Name, ".tar.gz")+"/playnux", next); err != nil {
		return err
	}
	verify, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	output, err := exec.CommandContext(verify, next, "--version").Output()
	if err != nil || strings.TrimSpace(string(output)) != "Playnux "+version {
		return fmt.Errorf("the downloaded executable does not report the requested version")
	}
	previous := filepath.Join(staging, "previous")
	if err := os.Link(binary, previous); err != nil {
		return err
	}
	if err := os.Rename(previous, binary+".previous"); err != nil {
		return err
	}
	return os.Rename(next, binary)
}

func checkReleaseRedirect(req *http.Request, via []*http.Request) error {
	if len(via) < 4 && req.URL.Scheme == "https" && req.URL.User == nil && req.URL.Port() == "" {
		switch req.URL.Hostname() {
		case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			return nil
		}
	}
	return fmt.Errorf("untrusted or excessive download redirects")
}

func download(ctx context.Context, client *http.Client, asset releaseAsset, staging string) (*os.File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxDownload {
		return nil, fmt.Errorf("could not download the update archive")
	}
	archive, err := os.Create(filepath.Join(staging, "archive.tar.gz"))
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	bytes, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, maxDownload+1))
	if err != nil || bytes > maxDownload || !strings.EqualFold("sha256:"+hex.EncodeToString(hash.Sum(nil)), asset.Digest) {
		archive.Close()
		return nil, fmt.Errorf("the update archive failed integrity verification")
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		archive.Close()
		return nil, err
	}
	return archive, nil
}

func unpackBinary(archive io.Reader, member, destination string) error {
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer compressed.Close()
	entries := tar.NewReader(io.LimitReader(compressed, maxDownload+1))
	for {
		header, err := entries.Next()
		if err == io.EOF {
			return fmt.Errorf("Playnux executable is missing from the archive")
		}
		if err != nil {
			return err
		}
		if header.Name != member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxDownload {
			return fmt.Errorf("invalid executable in the update archive")
		}
		file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(file, entries)
		if err != nil {
			file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}
}
