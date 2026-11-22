package anticheat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"name":"Blocked Online",
				"status":"Denied",
				"anticheats":["Vanguard"],
				"notes":[["Linux is not supported online","https://example.com"]],
				"storeIds":{"steam":"100"},
				"slug":"blocked-online"
			},
			{
				"name":"Fine Game",
				"status":"Supported",
				"anticheats":["Easy Anti-Cheat"],
				"notes":[],
				"storeIds":{"steam":"200"},
				"slug":"fine-game"
			},
			{
				"name":"Epic Online Game",
				"status":"Broken",
				"anticheats":["BattlEye"],
				"storeIds":{"epic":{"namespace":"abc","slug":"epic-online-game"}},
				"slug":"epic-online-game"
			}
		]`))
	}))
}

func TestLookupMatchesSteamIDAndFlagsDenied(t *testing.T) {
	server := testCatalog(t)
	defer server.Close()
	client := NewClient()
	client.CatalogURL = server.URL

	info, err := client.Lookup(context.Background(), "Blocked Online", "steam", "100")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Found || !info.Warning || info.Risk != "danger" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.AntiCheats) != 1 || info.AntiCheats[0] != "Vanguard" {
		t.Fatalf("anticheats = %v", info.AntiCheats)
	}
	if len(info.Notes) != 1 || info.Notes[0] == "" {
		t.Fatalf("notes = %v", info.Notes)
	}
}

func TestLookupFallsBackToTitleAndSupportsSafeStatus(t *testing.T) {
	server := testCatalog(t)
	defer server.Close()
	client := NewClient()
	client.CatalogURL = server.URL

	info, err := client.Lookup(context.Background(), "Fine Game™", "gog", "unknown")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Found || info.Warning || info.Risk != "safe" {
		t.Fatalf("info = %+v", info)
	}

	missing, err := client.Lookup(context.Background(), "Not In Catalog", "steam", "404")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Found {
		t.Fatalf("unexpected match: %+v", missing)
	}

	epic, err := client.Lookup(context.Background(), "Epic Online Game", "epic", "epic-online-game")
	if err != nil {
		t.Fatal(err)
	}
	if !epic.Found || !epic.Warning || epic.Risk != "warning" {
		t.Fatalf("epic info = %+v", epic)
	}
}
