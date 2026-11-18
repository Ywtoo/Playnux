package steamgriddb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupPicksHighestScoresAndPrefers600x900(t *testing.T) {
	mux := http.NewServeMux()
	checkAuth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer key" {
			http.Error(w, "sem auth", http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/search/autocomplete/Hades", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":7,"name":"Hades"}]}`))
	})
	mux.HandleFunc("/grids/game/7", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`{"success":true,"page":1,"limit":2,"total":3,"data":[{"url":"https://cdn/cover.jpg","width":600,"height":900,"score":12}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"page":0,"limit":2,"total":3,"data":[{"url":"https://cdn/wide.jpg","width":920,"height":430,"score":100},{"url":"https://cdn/low.jpg","width":600,"height":900,"score":3}]}`))
	})
	mux.HandleFunc("/heroes/game/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":[{"url":"https://cdn/other-hero.jpg","score":-1},{"url":"https://cdn/hero.jpg","score":4}]}`))
	})
	mux.HandleFunc("/logos/game/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":[{"url":"https://cdn/other-logo.png","score":1},{"url":"https://cdn/logo.png","score":2}]}`))
	})

	mux.HandleFunc("/icons/game/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":[{"url":"https://cdn/other-icon.png","score":1},{"url":"https://cdn/icon.png","thumb":"https://cdn/thumb-icon.png","score":5}]}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewSteamGridDB("key")
	client.base = server.URL

	art, err := client.Lookup(context.Background(), "Hades")
	if err != nil {
		t.Fatal(err)
	}
	if art.Cover != "https://cdn/cover.jpg" {
		t.Fatalf("capa = %q", art.Cover)
	}
	if art.Background != "https://cdn/hero.jpg" || art.Logo != "https://cdn/logo.png" || art.Icon != "https://cdn/icon.png" {
		t.Fatalf("art = %+v", art)
	}
	choices, err := client.SearchImages(context.Background(), "Hades", "icon")
	if err != nil || len(choices) != 2 || choices[0].URL != "https://cdn/icon.png" ||
		choices[0].Thumb != "https://cdn/thumb-icon.png" {
		t.Fatalf("miniaturas = %+v, erro = %v", choices, err)
	}
}

func TestLookupWithoutKey(t *testing.T) {
	client := NewSteamGridDB("")
	if _, err := client.Lookup(context.Background(), "Hades"); err == nil {
		t.Fatal("esperava erro sem chave")
	}
}
