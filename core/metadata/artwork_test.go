package metadata

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtworkCacheServesOnlyValidatedImagePaths(t *testing.T) {
	core := &ArtworkManager{Dir: t.TempDir()}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path, err := core.StoreArtwork(data.Bytes(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	handler := core.ServeArtwork(http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data.Bytes()) {
		t.Fatalf("image response: %d", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/artwork/../playnux.db", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected path response: %d", response.Code)
	}
}
