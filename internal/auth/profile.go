package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/user"
)

// Only bounded, decoded raster images are stored; re-encoding strips metadata.
func normalizeAvatar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	invalid := domain.Validation("Выберите корректное фото JPG или PNG размером до 256 × 256")
	header, encoded, ok := strings.Cut(value, ",")
	if !ok || (header != "data:image/jpeg;base64" && header != "data:image/png;base64") || len(encoded) > base64.StdEncoding.EncodedLen(40<<10) {
		return "", invalid
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", invalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 256 || config.Height > 256 || header != "data:image/"+format+";base64" {
		return "", invalid
	}
	picture, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", invalid
	}
	canvas := image.NewRGBA(picture.Bounds())
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), picture, picture.Bounds().Min, draw.Over)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 80}); err != nil {
		return "", invalid
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(output.Bytes()), nil
}

func (s *Service) UpdateProfile(ctx context.Context, uid string, name, avatar *string) (*user.User, error) {
	if name == nil && avatar == nil {
		return nil, domain.Validation("Укажите имя или фото")
	}
	trimmed := ""
	if name != nil {
		var err error
		trimmed, err = normalizeName(*name)
		if err != nil {
			return nil, err
		}
	}
	if avatar != nil {
		value, err := normalizeAvatar(*avatar)
		if err != nil {
			return nil, err
		}
		avatar = &value
	}
	return s.Users.Update(ctx, uid, trimmed, avatar, false)
}
