package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

type Case struct {
	Text     string         `json:"text"`
	Expected map[string]any `json:"expected"`
}
type Step struct {
	Method, Path   string
	Body           map[string]any
	Expected       map[string]any
	Status         int
	Restart        bool
	MinutesLater   int    `json:"minutes_later"`
	ExpectedDetail string `json:"expected_detail"`
}
type Scenario struct {
	Name  string
	Steps []Step
}
type Corpus struct {
	Clock, Timezone    string
	Temporal, Commands []Case
	Scenarios          []Scenario
}

func root() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
}
func load(t *testing.T) Corpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root(), "testdata/parity/corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus Corpus
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func compare(t *testing.T, path string, expected, actual any) {
	t.Helper()
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s: expected object, got %T", path, actual)
		}
		for key, v := range e {
			value, exists := a[key]
			if !exists {
				t.Fatalf("%s.%s missing", path, key)
			}
			compare(t, path+"."+key, v, value)
		}
	case []any:
		a, ok := actual.([]any)
		if !ok || len(a) != len(e) {
			t.Fatalf("%s: array size mismatch: expected %d got %v", path, len(e), actual)
		}
		for i, v := range e {
			compare(t, fmt.Sprintf("%s[%d]", path, i), v, a[i])
		}
	default:
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("%s: expected %#v got %#v", path, expected, actual)
		}
	}
}

// Keep the Python corpus immutable while explicitly tracking the two Go copy
// changes: selecting by name/date and no longer asking users to remember IDs.
// All other fields, history entries and reply text are still compared exactly.
func currentDialogueCopy(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			if text, ok := item.(string); key == "reply" && ok {
				text = strings.Replace(text, "Уточните номер записи:\n", "Подходят несколько записей. Уточните название, дату или время:\n", 1)
				if text == "Не нашёл запись для этого действия. Укажите название или номер, например #42." {
					text = "Не нашёл подходящую запись. Укажите название и, если нужно, дату: например, «удали напоминание позвонить маме завтра»."
				}
				if strings.HasPrefix(text, "Я помогу с задачами") {
					text = strings.Replace(text, "Я помогу", "Я Гена. Помогу", 1)
				}
				result[key] = text
			} else {
				result[key] = currentDialogueCopy(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = currentDialogueCopy(item)
		}
		return result
	}
	return value
}
func TestSharedCorpusDates(t *testing.T) {
	corpus := load(t)
	now, err := time.Parse(time.RFC3339, corpus.Clock)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation(corpus.Timezone)
	now = now.In(loc)
	for _, test := range corpus.Temporal {
		t.Run(test.Text, func(t *testing.T) {
			v := nlp.Extract(test.Text, now)
			var errorText any
			if v.Error != "" {
				errorText = v.Error
			}
			actual := map[string]any{"date": v.Date, "time": v.Time, "start": v.Start, "end": v.End, "duration": int(v.Duration / time.Minute), "error": errorText}
			compare(t, "temporal", test.Expected, jsonValue(t, actual))
		})
	}
}
func TestSharedCorpusCommands(t *testing.T) {
	corpus := load(t)
	now, _ := time.Parse(time.RFC3339, corpus.Clock)
	loc, _ := time.LoadLocation(corpus.Timezone)
	now = now.In(loc)
	for _, test := range corpus.Commands {
		t.Run(test.Text, func(t *testing.T) { compare(t, "command", test.Expected, jsonValue(t, nlp.Parse(test.Text, now))) })
	}
}
func TestSharedCorpusHTTP(t *testing.T) {
	corpus := load(t)
	base, _ := time.Parse(time.RFC3339, corpus.Clock)
	loc, _ := time.LoadLocation(corpus.Timezone)
	base = base.In(loc)
	engine, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range corpus.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "parity.sqlite3")
			now := base
			var db *sqlite.Database
			var handler http.Handler
			open := func() {
				var err error
				db, err = sqlite.Open(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				cal := &calendar.Service{Store: db, Search: engine, Location: loc, DemoPath: filepath.Join(root(), "data/voice_assistant_events_1000.csv")}
				a := &api.API{Config: config.Config{Root: root(), Timezone: corpus.Timezone, NLPProvider: "rules"}, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Assistant: assistant.New(db, nlp.Hybrid{}, cal), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
				handler = a.Router()
			}
			open()
			defer func() { db.Close() }()
			for i, step := range scenario.Steps {
				if step.Restart {
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					open()
				}
				now = base.Add(time.Duration(step.MinutesLater) * time.Minute)
				body, _ := json.Marshal(step.Body)
				request := httptest.NewRequest(step.Method, step.Path, bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != step.Status {
					t.Fatalf("step %d %s: expected status %d got %d %s", i, step.Path, step.Status, response.Code, response.Body.String())
				}
				var actual map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &actual); err != nil {
					t.Fatal(err)
				}
				if step.Expected != nil {
					compare(t, fmt.Sprintf("step%d", i), currentDialogueCopy(step.Expected), actual)
				}
				if step.ExpectedDetail != "" {
					compare(t, "detail", step.ExpectedDetail, actual["detail"])
				}
			}
		})
	}
}
