package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type WebImage struct {
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

const GoogleImagePairPattern = `\[\s*("https://encrypted-tbn[^"\\]*(?:\\.[^"\\]*)*")\s*,\s*[0-9]+\s*,\s*[0-9]+\s*\]\s*,\s*\[\s*("https?://[^"\\]*(?:\\.[^"\\]*)*")\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*\]`

var googleImagePair = regexp.MustCompile(GoogleImagePairPattern)
var googleLegacyImage = regexp.MustCompile(`(?is)<(?:div|span)[^>]*class=["'][^"']*\brg_meta\b[^"']*["'][^>]*>(\{.*?\})\s*</(?:div|span)>`)

func publicImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil &&
		((u.Scheme == "https" && (u.Port() == "" || u.Port() == "443")) ||
			(u.Scheme == "http" && (u.Port() == "" || u.Port() == "80")))
}

func googleThumbnail(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && publicImageURL(raw) && u.Scheme == "https" &&
		strings.HasPrefix(u.Hostname(), "encrypted-tbn") && strings.HasSuffix(u.Hostname(), ".gstatic.com")
}

func ParseGoogleImages(page string) []WebImage {
	results := make([]WebImage, 0, 24)
	seen := make(map[string]bool)
	add := func(item WebImage) {
		if len(results) < 24 && googleThumbnail(item.Thumbnail) && publicImageURL(item.URL) && !seen[item.URL] {
			seen[item.URL] = true
			results = append(results, item)
		}
	}
	for _, match := range googleImagePair.FindAllStringSubmatch(page, 96) {
		var item WebImage
		if json.Unmarshal([]byte(match[1]), &item.Thumbnail) != nil || json.Unmarshal([]byte(match[2]), &item.URL) != nil {
			continue
		}
		item.Height, _ = strconv.Atoi(match[3])
		item.Width, _ = strconv.Atoi(match[4])
		add(item)
	}
	for _, match := range googleLegacyImage.FindAllStringSubmatch(page, 48) {
		var legacy struct {
			URL       string `json:"ou"`
			Thumbnail string `json:"tu"`
			Width     int    `json:"ow"`
			Height    int    `json:"oh"`
		}
		if json.Unmarshal([]byte(html.UnescapeString(match[1])), &legacy) == nil {
			add(WebImage{URL: legacy.URL, Thumbnail: legacy.Thumbnail, Width: legacy.Width, Height: legacy.Height})
		}
	}
	return results
}

func GoogleImageSearchURL(query string, transparent bool) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 400 {
		return "", fmt.Errorf("informe uma busca de até 400 bytes")
	}
	params := url.Values{"udm": {"2"}, "q": {query}, "safe": {"active"}, "hl": {"pt-BR"}}
	if transparent {
		params.Set("tbs", "ic:trans")
	}
	return "https://www.google.com/search?" + params.Encode(), nil
}

func (m *ArtworkManager) ApplyWebImage(ctx context.Context, id, field string, result WebImage) error {
	if !publicImageURL(result.URL) || (result.Thumbnail != "" && !googleThumbnail(result.Thumbnail)) {
		return fmt.Errorf("resultado de imagem inválido")
	}
	for _, source := range []string{result.URL, result.Thumbnail} {
		path, err := m.DownloadWebImage(ctx, source)
		if err == nil {
			return m.setArtwork(ctx, id, field, path)
		}
		if result.Thumbnail == "" || result.URL == result.Thumbnail {
			break
		}
	}
	if result.Thumbnail == "" {
		return fmt.Errorf("não foi possível baixar a imagem desse endereço")
	}
	return fmt.Errorf("não foi possível salvar a imagem original nem a miniatura")
}

func (m *ArtworkManager) DownloadWebImage(ctx context.Context, source string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}
	resp, err := webImageClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imagem indisponível")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtworkBytes+1))
	if err != nil || len(data) > maxArtworkBytes {
		return "", fmt.Errorf("imagem ilegível ou maior que 3 MB")
	}
	return m.StoreArtwork(data, "manual")
}

var sharedIPv4 = netip.MustParsePrefix("100.64.0.0/10")
var specialIPv4 = netip.MustParsePrefix("192.0.0.0/24")
var benchmarkIPv4 = netip.MustParsePrefix("198.18.0.0/15")
var globalIPv6 = netip.MustParsePrefix("2000::/3")
var tunnelIPv6 = netip.MustParsePrefix("2002::/16")
var transitionIPv6 = netip.MustParsePrefix("2001::/23")

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is4() {
		return !sharedIPv4.Contains(ip) && !specialIPv4.Contains(ip) && !benchmarkIPv4.Contains(ip)
	}
	return globalIPv6.Contains(ip) && !tunnelIPv6.Contains(ip) && !transitionIPv6.Contains(ip)
}

var webImageClient = &http.Client{
	Timeout: 12 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 || !publicImageURL(req.URL.String()) {
			return http.ErrUseLastResponse
		}
		return nil
	},
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			dialer := net.Dialer{Timeout: 4 * time.Second}
			for _, address := range addresses {
				if !publicAddress(address) {
					continue
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, fmt.Errorf("endereço de imagem indisponível")
		},
	},
}
