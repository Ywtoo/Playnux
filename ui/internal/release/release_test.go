package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckFindsTheLatestCompatibleReleaseAndVerifiesTheAssetIdentity(t *testing.T) {
	name := "playnux-0.3.0-alpha-linux-" + runtime.GOARCH + ".tar.gz"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[
			{"tag_name":"v9.0.0","draft":true},
			{"tag_name":"v0.2.0"},
			{"tag_name":"v0.3.0-alpha","prerelease":true,"assets":[
				{"name":%q,"digest":"sha256:%s","browser_download_url":%q}
			]}
		]`, name, strings.Repeat("ab", 32), releasesURL+"/download/v0.3.0-alpha/"+name)
	}))
	defer server.Close()
	alpha, err := check(context.Background(), server.Client(), server.URL, "0.1.0-alpha")
	if err != nil || !alpha.Available || alpha.Latest != "0.3.0-alpha" || alpha.asset.Name != name {
		t.Fatalf("alpha release: %+v (%v)", alpha, err)
	}
	stable, err := check(context.Background(), server.Client(), server.URL, "0.1.0")
	if err != nil || !stable.Available || stable.Latest != "0.2.0" || stable.asset.URL != "" {
		t.Fatalf("stable release: %+v (%v)", stable, err)
	}
}

func TestInstallBinaryKeepsTheLinkAndPreviousExecutable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux executable fixture")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "playnux")
	link := filepath.Join(directory, "launcher")
	const previous = "#!/bin/sh\nprintf 'Playnux 0.1.0-alpha\\n'\n"
	const next = "#!/bin/sh\nprintf 'Playnux 0.1.1-alpha\\n'\n"
	if err := os.WriteFile(binary, []byte(previous), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	tarball := tar.NewWriter(compressed)
	const archiveName = "playnux-fixture.tar.gz"
	if err := tarball.WriteHeader(&tar.Header{Name: "playnux-fixture/playnux", Size: int64(len(next)), Mode: 0o755, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarball.Write([]byte(next)); err != nil {
		t.Fatal(err)
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	asset := releaseAsset{URL: server.URL, Name: archiveName, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(archive.Bytes()))}
	if err := installBinary(context.Background(), server.Client(), binary, "0.1.1-alpha", asset); err != nil {
		t.Fatal(err)
	}
	if actual, err := os.Readlink(link); err != nil || actual != binary {
		t.Fatalf("launcher link: %q (%v)", actual, err)
	}
	output, err := exec.Command(link, "--version").Output()
	if err != nil || strings.TrimSpace(string(output)) != "Playnux 0.1.1-alpha" {
		t.Fatalf("updated version: %q (%v)", output, err)
	}
	if data, err := os.ReadFile(binary + ".previous"); err != nil || string(data) != previous {
		t.Fatalf("previous binary: %q (%v)", data, err)
	}
}

func TestDownloadRejectsAnUnexpectedChecksum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "unexpected archive content")
	}))
	defer server.Close()
	file, err := download(context.Background(), server.Client(), releaseAsset{
		URL: server.URL, Digest: "sha256:" + strings.Repeat("00", 32),
	}, t.TempDir())
	if err == nil || file != nil {
		t.Fatal("accepted archive with mismatched checksum")
	}
}
