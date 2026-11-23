package steam

import (
	"reflect"
	"strings"
	"testing"
)

func TestTrailerSourcesUsesHighlightedHLS(t *testing.T) {
	input := `{"123":{"success":true,"data":{"movies":[
		{"mp4":{"480":"https://cdn.example/other.mp4"}},
		{"highlight":true,"hls_h264":"https://cdn.example/main.m3u8?quality=auto"}
	]}}}`
	got := parseTrailerSources(strings.NewReader(input), "123", true)
	if len(got) != 1 || got[0] != "https://cdn.example/main.m3u8?quality=auto" {
		t.Fatalf("trailer = %v", got)
	}
}

func TestTrailerSourcesPrefersMaximumQualityForBigPicture(t *testing.T) {
	const input = `{"123":{"success":true,"data":{"movies":[{
		"mp4":{"480":"https://cdn.example/low.mp4","max":"https://cdn.example/max.mp4"},
		"webm":{"480":"https://cdn.example/low.webm","max":"https://cdn.example/max.webm"},
		"hls_h264":"https://cdn.example/master.m3u8"
	}]}}}`
	want := []string{
		"https://cdn.example/max.mp4", "https://cdn.example/max.webm", "https://cdn.example/master.m3u8",
		"https://cdn.example/low.mp4", "https://cdn.example/low.webm",
	}
	if got := parseTrailerSources(strings.NewReader(input), "123", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("ordem do Big Picture = %v", got)
	}
	if got := parseTrailerSources(strings.NewReader(input), "123", false); len(got) != 5 || got[0] != "https://cdn.example/low.mp4" {
		t.Fatalf("ordem de Detalhes = %v", got)
	}
}
