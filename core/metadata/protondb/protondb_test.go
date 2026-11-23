package protondb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDerivesRecommendationFromRecentPositiveReports(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/summary/123.json" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.URL.Path != "/games/123/reports/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"rating":"borked","protonVersion":"Proton 8.0","timestamp":500},
			{"rating":"gold","protonVersion":"Proton 9.0-3","timestamp":"400"},
			{"rating":"platinum","protonVersion":"GE-Proton 9-24","timestamp":300},
			{"rating":"platinum","protonVersion":"GE-Proton 9-24","timestamp":200}
		]`))
	}))
	defer server.Close()

	client := NewClient()
	client.ReportsURL = server.URL + "/games/%s/reports/"
	client.SummaryURL = server.URL + "/summary/%s.json"
	intel, err := client.Fetch(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if !intel.Available || intel.Tier != "platinum" {
		t.Fatalf("intelligence = %+v", intel)
	}
	if intel.Recommended != "GE-Proton 9-24" || intel.RecommendedTier != "platinum" || intel.PositiveReports != 2 {
		t.Fatalf("recommendation = %q (%s)", intel.Recommended, intel.RecommendedTier)
	}
	if intel.TotalReports != 4 || len(intel.Recent) != 4 {
		t.Fatalf("reports = %d recent=%d", intel.TotalReports, len(intel.Recent))
	}
	if intel.Recent[0].Rating != "borked" || intel.Recent[0].Timestamp != 500 {
		t.Fatalf("recent = %+v", intel.Recent)
	}
	if intel.URL != "https://www.protondb.com/app/123" {
		t.Fatalf("url = %q", intel.URL)
	}
}

func TestFetchUsesFallbackReportsWhenLegacyDatasetIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/games/555/reports/":
			_, _ = w.Write([]byte(`[]`))
		case "/pulse/555/latest.json":
			_, _ = w.Write([]byte(`[{"rating":"platinum","protonVersion":"GE-Proton 9-24","timestamp":100}]`))
		case "/summary/555.json":
			_, _ = w.Write([]byte(`{"tier":"native"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.ReportsURL = server.URL + "/games/%s/reports/"
	client.FallbackReportsURL = server.URL + "/pulse/%s/latest.json"
	client.SummaryURL = server.URL + "/summary/%s.json"
	intel, err := client.Fetch(context.Background(), "555")
	if err != nil {
		t.Fatal(err)
	}
	if intel.Tier != "native" || !intel.NativeAvailable || intel.Recommended != "GE-Proton 9-24" {
		t.Fatalf("intelligence = %+v", intel)
	}
}

func TestFetchFallsBackToSummaryWhenReportsAreUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/games/999/reports/", "/pulse/999/latest.json":
			http.Error(w, "not found", http.StatusNotFound)
		case "/summary/999.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tier":"silver","confidence":"moderate","total":12}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient()
	client.ReportsURL = server.URL + "/games/%s/reports/"
	client.FallbackReportsURL = server.URL + "/pulse/%s/latest.json"
	client.SummaryURL = server.URL + "/summary/%s.json"
	intel, err := client.Fetch(context.Background(), "999")
	if err != nil {
		t.Fatal(err)
	}
	if !intel.Available || intel.Tier != "silver" || intel.TotalReports != 12 {
		t.Fatalf("intelligence = %+v", intel)
	}
	if intel.Recommended != "" || intel.Basis == "" {
		t.Fatalf("expected no recommendation with explicit fallback basis: %+v", intel)
	}
}
