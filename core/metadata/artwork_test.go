package metadata

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestArtworkCacheServesWebP(t *testing.T) {
	manager := &ArtworkManager{Dir: t.TempDir()}
	data, err := base64.StdEncoding.DecodeString("UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoCAAIAAgA0JaACdLoB+AADsAD+8Oj3/yC5YXXI1/8gP+QH/ID/+PIAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	path, err := manager.StoreArtwork(data, "manual")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	manager.ServeArtwork(http.NotFoundHandler()).ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/webp" || !bytes.Equal(response.Body.Bytes(), data) {
		t.Fatalf("resposta da imagem WebP: %d %s", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestLogoThumbnailFitsVisibleArtwork(t *testing.T) {
	manager := &ArtworkManager{Dir: t.TempDir()}
	logo := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	content := image.Rect(100, 140, 500, 260)
	draw.Draw(logo, content, image.NewUniform(color.White), image.Point{}, draw.Src)
	var original bytes.Buffer
	if err := png.Encode(&original, logo); err != nil {
		t.Fatal(err)
	}
	source, err := manager.StoreArtwork(original.Bytes(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	handler := manager.ServeArtwork(http.NotFoundHandler())
	for _, check := range []struct {
		role string
		size image.Point
	}{
		{"logo", content.Size()},
		{"banner", logo.Bounds().Size()},
	} {
		response := httptest.NewRecorder()
		path := thumbPrefix + check.role + "?src=" + url.QueryEscape(source)
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		config, _, err := image.DecodeConfig(response.Body)
		if response.Code != http.StatusOK || err != nil || image.Pt(config.Width, config.Height) != check.size {
			t.Fatalf("%s: status %d, imagem %dx%d, erro %v", check.role, response.Code, config.Width, config.Height, err)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, source, nil))
	if !bytes.Equal(response.Body.Bytes(), original.Bytes()) {
		t.Fatal("o logo original foi alterado")
	}
}

func TestThumbnailPreservesOriginalAndUsesSeparateAppropriateSize(t *testing.T) {
	manager := &ArtworkManager{Dir: t.TempDir()}
	original := image.NewNRGBA(image.Rect(0, 0, 1200, 600))
	var buf bytes.Buffer
	if err := png.Encode(&buf, original); err != nil {
		t.Fatal(err)
	}
	source, err := manager.StoreArtwork(buf.Bytes(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	handler := manager.ServeArtwork(http.NotFoundHandler())
	thumbURL := thumbPrefix + "cover?src=" + url.QueryEscape(source)
	for range []int{1, 2} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, thumbURL, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("thumbnail status = %d", response.Code)
		}
		image, format, err := image.DecodeConfig(response.Body)
		if err != nil || format != "png" || image.Width != 250 || image.Height != 125 {
			t.Fatalf("thumbnail config = %+v %s %v", image, format, err)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, source, nil))
	if !bytes.Equal(response.Body.Bytes(), buf.Bytes()) {
		t.Fatal("a imagem original foi alterada")
	}
}

func TestBigPictureBannerPreservesOriginalJPEG(t *testing.T) {
	manager := &ArtworkManager{Dir: t.TempDir()}
	var original bytes.Buffer
	if err := jpeg.Encode(&original, image.NewRGBA(image.Rect(0, 2560, 2560, 3840)), &jpeg.Options{Quality: 96}); err != nil {
		t.Fatal(err)
	}
	source, err := manager.StoreArtwork(original.Bytes(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	handler := manager.ServeArtwork(http.NotFoundHandler())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, thumbPrefix+"big-banner?src="+url.QueryEscape(source), nil))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), original.Bytes()) {
		t.Fatalf("banner original do Big Picture: status %d", response.Code)
	}
}
