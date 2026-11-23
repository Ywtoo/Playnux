// Package protondb reads community reports from ProtonDB and derives the best
// Proton version for a Steam game. It is intentionally network-only and does
// not depend on the database or the UI.
package protondb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	reportsURL        = "https://www.proton-pulse.com/data/%s/latest.json"
	fallbackReportURL = "https://protondb.max-p.me/games/%s/reports/"
	summaryURL        = "https://www.protondb.com/api/v1/reports/summaries/%s.json"
	cacheTTL          = 15 * time.Minute
)

// Report is one community compatibility report.
type Report struct {
	Rating        string `json:"rating"`
	ProtonVersion string `json:"protonVersion"`
	Timestamp     int64  `json:"timestamp"`
}

// RecentReport is a compact report chip for the UI.
type RecentReport struct {
	Rating        string `json:"rating"`
	ProtonVersion string `json:"proton_version"`
	Timestamp     int64  `json:"timestamp"`
}

// Intelligence is the derived ProtonDB summary shown in game details.
type Intelligence struct {
	AppID           string         `json:"app_id"`
	Available       bool           `json:"available"`
	Tier            string         `json:"tier"`
	Confidence      string         `json:"confidence,omitempty"`
	TotalReports    int            `json:"total_reports"`
	Recommended     string         `json:"recommended,omitempty"`
	RecommendedTier string         `json:"recommended_tier,omitempty"`
	SampleSize      int            `json:"sample_size,omitempty"`
	PositiveReports int            `json:"positive_reports,omitempty"`
	NativeAvailable bool           `json:"native_available,omitempty"`
	Basis           string         `json:"basis,omitempty"`
	Recent          []RecentReport `json:"recent,omitempty"`
	URL             string         `json:"url"`
	Error           string         `json:"error,omitempty"`
}

type summary struct {
	Tier             string `json:"tier"`
	ProvisionalTier  string `json:"provisionalTier"`
	BestReportedTier string `json:"bestReportedTier"`
	TrendingTier     string `json:"trendingTier"`
	Confidence       string `json:"confidence"`
	Total            int    `json:"total"`
}

type cached struct {
	value   Intelligence
	expires time.Time
}

// Client fetches and caches ProtonDB intelligence.
type Client struct {
	ReportsURL         string
	FallbackReportsURL string
	SummaryURL         string
	HTTP               *http.Client

	mu    sync.Mutex
	cache map[string]cached
}

func NewClient() *Client {
	return &Client{
		ReportsURL:         reportsURL,
		FallbackReportsURL: fallbackReportURL,
		SummaryURL:         summaryURL,
		HTTP:               &http.Client{Timeout: 10 * time.Second},
		cache:              make(map[string]cached),
	}
}

// Fetch returns the ProtonDB intelligence for a Steam AppID.
func (c *Client) Fetch(ctx context.Context, appID string) (Intelligence, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return Intelligence{}, fmt.Errorf("protondb: empty app id")
	}
	if c == nil {
		return Intelligence{}, fmt.Errorf("protondb: client is not configured")
	}

	c.mu.Lock()
	if entry, ok := c.cache[appID]; ok && time.Now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.value, nil
	}
	c.mu.Unlock()

	intel := Intelligence{
		AppID: appID,
		URL:   "https://www.protondb.com/app/" + appID,
	}
	type summaryResponse struct {
		value summary
		err   error
	}
	summaries := make(chan summaryResponse, 1)
	go func() {
		value, err := c.fetchSummary(ctx, appID)
		summaries <- summaryResponse{value, err}
	}()
	reports, reportsErr := c.fetchReports(ctx, c.ReportsURL, appID)
	if len(reports) == 0 && c.FallbackReportsURL != "" {
		// Prefer the monthly mirror. The legacy mirror can still cover older titles.
		if fallback, fallbackErr := c.fetchReports(ctx, c.FallbackReportsURL, appID); len(fallback) > 0 {
			reports = fallback
			reportsErr = nil
		} else if fallbackErr != nil && reportsErr == nil {
			reportsErr = fallbackErr
		}
	}
	if len(reports) > 0 {
		derive(&intel, reports)
	}
	storeSummary := <-summaries
	if storeSummary.err == nil {
		value := storeSummary.value
		officialTier := normalizedTier(value.Tier)
		if officialTier == "" || officialTier == "pending" {
			if provisional := normalizedTier(value.ProvisionalTier); provisional != "" {
				officialTier = provisional
			}
		}
		if officialTier == "" {
			officialTier = normalizedTier(value.BestReportedTier)
		}
		if officialTier != "" {
			intel.Available = true
			intel.Tier = officialTier
			intel.NativeAvailable = intel.NativeAvailable || officialTier == "native"
			if value.Confidence != "" {
				intel.Confidence = value.Confidence
			}
		}
		intel.TotalReports = max(intel.TotalReports, value.Total)
	}
	if !intel.Available && storeSummary.err != nil {
		if reportsErr != nil {
			return Intelligence{}, reportsErr
		}
		return Intelligence{}, storeSummary.err
	}
	if intel.Basis == "" {
		intel.Basis = "Os relatos disponíveis não indicam uma versão específica do Proton."
	}

	c.mu.Lock()
	c.cache[appID] = cached{value: intel, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()
	return intel, nil
}

func (c *Client) fetchReports(ctx context.Context, template, appID string) ([]Report, error) {
	endpoint := fmt.Sprintf(template, appID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Playnux/1.0 (+https://github.com/playnux)")
	request.Header.Set("Accept", "application/json")

	response, err := c.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("protondb: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("protondb: reports returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 || strings.TrimSpace(string(body)) == "null" {
		return nil, nil
	}
	var reports []Report
	if err := json.Unmarshal(body, &reports); err != nil {
		return nil, fmt.Errorf("protondb: decode reports: %w", err)
	}
	return reports, nil
}

func (c *Client) fetchSummary(ctx context.Context, appID string) (summary, error) {
	endpoint := fmt.Sprintf(c.SummaryURL, appID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return summary{}, err
	}
	request.Header.Set("User-Agent", "Playnux/1.0 (+https://github.com/playnux)")
	request.Header.Set("Accept", "application/json")

	response, err := c.client().Do(request)
	if err != nil {
		return summary{}, fmt.Errorf("protondb: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return summary{}, fmt.Errorf("protondb: summary returned status %d", response.StatusCode)
	}
	var value summary
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value); err != nil {
		return summary{}, err
	}
	return value, nil
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func derive(intel *Intelligence, reports []Report) {
	filtered := make([]Report, 0, len(reports))
	for _, report := range reports {
		rating := normalizedTier(report.Rating)
		if rating == "" || rating == "pending" {
			continue
		}
		report.Rating = rating
		filtered = append(filtered, report)
	}
	if len(filtered) == 0 {
		return
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Timestamp > filtered[j].Timestamp })

	intel.Available = true
	intel.TotalReports = len(filtered)
	intel.Tier = winningTier(filtered)
	intel.Confidence = confidenceFor(len(filtered))
	intel.NativeAvailable = intel.Tier == "native"
	const sampleLimit = 10
	sample := filtered[:min(sampleLimit, len(filtered))]
	intel.SampleSize = len(sample)
	intel.Recent = make([]RecentReport, 0, len(sample))
	type versionScore struct {
		version  string
		tier     string
		positive int
		negative int
	}
	tallies := make(map[string]*versionScore)
	var ranked []*versionScore
	for _, report := range sample {
		intel.Recent = append(intel.Recent, RecentReport{
			Rating: report.Rating, ProtonVersion: report.ProtonVersion, Timestamp: report.Timestamp,
		})
		if report.Rating == "native" {
			intel.NativeAvailable = true
		}
		version := displayVersion(report.ProtonVersion)
		if version == "" {
			continue
		}
		key := strings.NewReplacer(" ", "", "-x86_64", "", "-arm64", "").Replace(strings.ToLower(version))
		entry := tallies[key]
		if entry == nil {
			entry = &versionScore{version: version}
			tallies[key] = entry
			ranked = append(ranked, entry)
		}
		switch report.Rating {
		case "platinum", "gold":
			entry.positive++
			if entry.tier == "" {
				entry.tier = report.Rating
			}
		case "bronze", "borked":
			entry.negative++
		}
	}
	var best *versionScore
	for _, candidate := range ranked {
		score := candidate.positive - candidate.negative
		if score > 0 && (best == nil || score > best.positive-best.negative ||
			(score == best.positive-best.negative && candidate.positive > best.positive)) {
			best = candidate
		}
	}
	if best != nil {
		intel.Recommended = best.version
		intel.RecommendedTier = best.tier
		intel.PositiveReports = best.positive
		intel.Basis = fmt.Sprintf("%d relato(s) Gold ou Platinum entre os %d mais recentes.", best.positive, len(sample))
	}

}

func winningTier(reports []Report) string {
	limit := len(reports)
	if limit > 30 {
		limit = 30
	}
	counts := make(map[string]int)
	for _, report := range reports[:limit] {
		counts[report.Rating]++
	}
	best := ""
	for tier, count := range counts {
		if best == "" || count > counts[best] || (count == counts[best] && tierRank(tier) > tierRank(best)) {
			best = tier
		}
	}
	return best
}

func confidenceFor(count int) string {
	switch {
	case count >= 50:
		return "strong"
	case count >= 20:
		return "high"
	case count >= 10:
		return "moderate"
	case count >= 5:
		return "low"
	default:
		return "inadequate"
	}
}

func tierRank(tier string) int {
	switch tier {
	case "native":
		return 6
	case "platinum":
		return 5
	case "gold":
		return 4
	case "silver":
		return 3
	case "bronze":
		return 2
	case "borked":
		return 1
	}
	return 0
}

func normalizedTier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "platinum", "gold", "silver", "bronze", "borked", "pending", "native":
		return value
	}
	return ""
}

var reportedProtonVersion = regexp.MustCompile(`(?i)^(?:ge[- ]proton[- ]*|proton[- ]*)?\d+(?:[.-]\d+)+(?:[a-z0-9.-]*)$`)

func displayVersion(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	switch lower {
	case "experimental", "proton experimental", "proton - experimental", "proton-experimental":
		return "Proton Experimental"
	case "hotfix", "proton hotfix", "proton - hotfix", "proton-hotfix":
		return "Proton Hotfix"
	}
	if !reportedProtonVersion.MatchString(value) {
		return ""
	}
	if value[0] >= '0' && value[0] <= '9' {
		return "Proton " + value
	}
	return value
}

// UnmarshalJSON accepts timestamps encoded as numbers or strings.
func (r *Report) UnmarshalJSON(data []byte) error {
	var raw struct {
		Rating        string          `json:"rating"`
		ProtonVersion string          `json:"protonVersion"`
		Timestamp     json.RawMessage `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Rating = raw.Rating
	r.ProtonVersion = raw.ProtonVersion
	r.Timestamp = parseTimestamp(raw.Timestamp)
	return nil
}

func parseTimestamp(raw json.RawMessage) int64 {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || value == "null" {
		return 0
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return int64(parsed)
	}
	return 0
}
