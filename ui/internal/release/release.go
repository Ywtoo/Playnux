package release

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const releasesURL = "https://github.com/Ywtoo/Playnux/releases"
const releasesAPI = "https://api.github.com/repos/Ywtoo/Playnux/releases?per_page=30"

//go:embed version.txt
var version string

func Version() string { return strings.TrimSpace(version) }

type Update struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	URL         string `json:"url"`
	Installable bool   `json:"installable"`
	asset       releaseAsset
}

type releaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type githubRelease struct {
	Tag        string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

func Check(ctx context.Context) (Update, error) {
	result, err := check(ctx, &http.Client{Timeout: 8 * time.Second}, releasesAPI, Version())
	_, managed := managedBinary()
	result.Installable = result.Available && managed && result.asset.URL != ""
	return result, err
}

func check(ctx context.Context, client *http.Client, endpoint, version string) (Update, error) {
	result := Update{Current: version, URL: releasesURL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Playnux/"+version)
	response, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("release service returned HTTP %d", response.StatusCode)
	}
	var entries []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&entries); err != nil {
		return result, err
	}
	current := "v" + version
	latest := ""
	for _, entry := range entries {
		candidate := "v" + strings.TrimPrefix(entry.Tag, "v")
		if entry.Draft || !semver.IsValid(candidate) || strings.Count(strings.SplitN(candidate, "-", 2)[0], ".") != 2 {
			continue
		}
		if semver.Prerelease(current) == "" && (entry.Prerelease || semver.Prerelease(candidate) != "") {
			continue
		}
		if semver.Compare(candidate, latest) > 0 {
			latest = candidate
			result.Latest = strings.TrimPrefix(candidate, "v")
			result.URL = releasesURL + "/tag/" + entry.Tag
			result.asset = releaseAsset{}
			name := "playnux-" + result.Latest + "-linux-" + runtime.GOARCH + ".tar.gz"
			for _, asset := range entry.Assets {
				if asset.Name == name && asset.URL == releasesURL+"/download/"+entry.Tag+"/"+name && validDigest(asset.Digest) {
					result.asset = asset
					break
				}
			}
		}
	}
	result.Available = semver.Compare(latest, current) > 0
	return result, nil
}
