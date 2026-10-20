package calendar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

const CSVMaxBytes = 5 << 20
const CSVMaxRows = 10000

// ImportCSV identifies a batch by its contents, never the client-supplied name.
// Preserve the existing demo namespace so uploading the supplied assignment CSV
// and loading the built-in examples share the same database uniqueness keys.
func (s *Service) ImportCSV(ctx context.Context, input io.Reader, now time.Time) (ImportResult, error) {
	data, err := io.ReadAll(io.LimitReader(input, CSVMaxBytes+1))
	if err != nil {
		return ImportResult{}, domain.Validation("Не удалось прочитать CSV")
	}
	if len(data) > CSVMaxBytes {
		return ImportResult{}, &domain.AppError{Status: 413, Code: "csv_too_large", Message: "CSV должен быть не больше 5 МБ"}
	}
	if !utf8.Valid(data) {
		return ImportResult{}, domain.Validation("Сохраните CSV в кодировке UTF-8")
	}
	fingerprint := csvFingerprint(data)
	namespace := fmt.Sprintf("upload-%x", fingerprint)
	if demo, readErr := os.ReadFile(s.DemoPath); readErr == nil && csvFingerprint(demo) == fingerprint {
		namespace = filepath.Base(s.DemoPath)
	}
	return s.Import(ctx, bytes.NewReader(data), namespace, now)
}

func csvFingerprint(data []byte) [32]byte {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return sha256.Sum256(data)
}
