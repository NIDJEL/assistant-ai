package calendar

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func (s *Service) ImportDemo(ctx context.Context, now time.Time) (ImportResult, error) {
	f, err := os.Open(s.DemoPath)
	if err != nil {
		return ImportResult{}, err
	}
	defer f.Close()
	return s.Import(ctx, f, filepath.Base(s.DemoPath), now)
}
