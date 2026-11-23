package metadata

import (
	"net/netip"
	"net/url"
	"testing"
)

func TestParseGoogleImages(t *testing.T) {
	page := `["https://encrypted-tbn0.gstatic.com/images?q=abc",90,160],["https://example.com/cover.jpg?x=1\u0026y=2",900,600]
<div class="rg_meta">{"ou":"https://example.com/icon.png","tu":"https://encrypted-tbn1.gstatic.com/images?q=icon","ow":32,"oh":32}</div>
<div class="rg_meta">{"ou":"https://example.com/unsafe.png","tu":"https://encrypted-tbn0.gstatic.com.example.com/icon.png"}</div>`
	results := ParseGoogleImages(page)
	if len(results) != 2 || results[0].Width != 600 || results[0].Height != 900 || results[0].URL != "https://example.com/cover.jpg?x=1&y=2" || results[1].Width != 32 {
		t.Fatalf("resultados: %+v", results)
	}
}

func TestGoogleImageSearchURLUsesImageModeAndTransparency(t *testing.T) {
	raw, err := GoogleImageSearchURL("  Hollow Knight logotipo  ", true)
	if err != nil {
		t.Fatal(err)
	}
	search, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	params := search.Query()
	if search.Hostname() != "www.google.com" || params.Get("udm") != "2" || params.Get("q") != "Hollow Knight logotipo" || params.Get("tbs") != "ic:trans" {
		t.Fatalf("busca incorreta: %s", raw)
	}
}

func TestPublicImageAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.100.100.200", "::1", "::ffff:192.168.1.1", "64:ff9b::a00:1", "2002:7f00:1::"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("aceitou endereço não público %s", address)
		}
	}
	if !publicAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("rejeitou endereço público")
	}
}
