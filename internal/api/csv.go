package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func (a *API) importCSV(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, code, message string) {
		a.fail(w, r, &domain.AppError{Status: status, Code: code, Message: message})
	}
	r.Body = http.MaxBytesReader(w, r.Body, calendar.CSVMaxBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		fail(400, "invalid_csv_upload", "Выберите один CSV-файл")
		return
	}
	var data []byte
	found := false
	for {
		part, readErr := reader.NextPart()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			var max *http.MaxBytesError
			if errors.As(readErr, &max) {
				fail(413, "csv_too_large", "CSV должен быть не больше 5 МБ")
			} else {
				fail(400, "invalid_csv_upload", "Не удалось прочитать CSV-файл")
			}
			return
		}
		if found || part.FormName() != "file" || part.FileName() == "" {
			fail(400, "invalid_csv_upload", "Выберите один CSV-файл")
			return
		}
		found = true
		if !strings.EqualFold(filepath.Ext(part.FileName()), ".csv") {
			fail(415, "unsupported_csv_file", "Выберите файл с расширением .csv")
			return
		}
		data, err = io.ReadAll(io.LimitReader(part, calendar.CSVMaxBytes+1))
		var max *http.MaxBytesError
		if len(data) > calendar.CSVMaxBytes || errors.As(err, &max) {
			fail(413, "csv_too_large", "CSV должен быть не больше 5 МБ")
			return
		}
		if err != nil {
			fail(400, "invalid_csv_upload", "Не удалось прочитать CSV-файл")
			return
		}
	}
	if len(data) == 0 {
		fail(400, "empty_csv", "CSV-файл пуст")
		return
	}
	// Validate the complete file and multipart body before opening a transaction.
	// Uploads stay in memory and never become files on the server.
	result, err := a.Calendar.ImportCSV(r.Context(), bytes.NewReader(data), a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
