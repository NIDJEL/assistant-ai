package auth

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

func avatarPNG(width, height int) string {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		panic(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw.Bytes())
}

func TestNormalizeAvatar(t *testing.T) {
	photo, err := normalizeAvatar(avatarPNG(256, 256))
	if err != nil || !strings.HasPrefix(photo, "data:image/jpeg;base64,") {
		t.Fatal("valid raster image rejected", err)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(photo, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != 256 || config.Height != 256 {
		t.Fatal("avatar dimensions changed", config, err)
	}
	if value, err := normalizeAvatar(""); err != nil || value != "" {
		t.Fatal("avatar removal rejected", err)
	}
	for _, value := range []string{"https://example.test/photo.jpg", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/jpeg;base64,bm90IGFuIGltYWdl", avatarPNG(257, 1), strings.Replace(avatarPNG(2, 2), "image/png", "image/jpeg", 1), "data:image/png;base64," + strings.Repeat("A", 60000)} {
		if _, err := normalizeAvatar(value); err == nil {
			t.Fatal("invalid or oversized avatar accepted")
		}
	}
}
