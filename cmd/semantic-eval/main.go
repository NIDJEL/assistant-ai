package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

type corpusCase struct {
	ID               string               `json:"id"`
	Text             string               `json:"text"`
	Source           string               `json:"source"`
	Tags             []string             `json:"tags"`
	Expected         expected             `json:"expected"`
	SemanticExpected *nlp.SemanticCommand `json:"semantic_expected,omitempty"`
}

type expected struct {
	Action          string                `json:"action"`
	Kind            *string               `json:"kind"`
	Title           string                `json:"title"`
	Query           string                `json:"query"`
	Date            *string               `json:"date"`
	Time            *string               `json:"time"`
	PeriodStart     *string               `json:"period_start"`
	PeriodEnd       *string               `json:"period_end"`
	ReminderMinutes *int                  `json:"reminder_minutes"`
	ReminderEnabled *bool                 `json:"reminder_enabled"`
	DurationMinutes int                   `json:"duration_minutes"`
	TargetID        *int64                `json:"target_id"`
	StartSecond     int                   `json:"start_second"`
	ErrorExpected   bool                  `json:"error_expected,omitempty"`
	Recurrence      map[string]any        `json:"recurrence,omitempty"`
	Selector        *domain.EventSelector `json:"selector,omitempty"`
}

type actual struct {
	Action          string                `json:"action"`
	Kind            *string               `json:"kind"`
	Title           string                `json:"title"`
	Query           string                `json:"query"`
	Date            *string               `json:"date"`
	Time            *string               `json:"time"`
	PeriodStart     *string               `json:"period_start"`
	PeriodEnd       *string               `json:"period_end"`
	ReminderMinutes *int                  `json:"reminder_minutes"`
	ReminderEnabled *bool                 `json:"reminder_enabled"`
	DurationMinutes int                   `json:"duration_minutes"`
	TargetID        *int64                `json:"target_id"`
	StartSecond     int                   `json:"start_second"`
	Error           string                `json:"error,omitempty"`
	Provider        string                `json:"provider,omitempty"`
	Recurrence      map[string]any        `json:"recurrence,omitempty"`
	Selector        *domain.EventSelector `json:"selector,omitempty"`
}

type result struct {
	ID                     string            `json:"id"`
	Text                   string            `json:"text"`
	Source                 string            `json:"source"`
	Tags                   []string          `json:"tags,omitempty"`
	Expected               expected          `json:"expected"`
	Actual                 actual            `json:"actual"`
	CommandCorrect         bool              `json:"command_correct"`
	FullCorrect            bool              `json:"full_parse_correct"`
	Unknown                bool              `json:"unknown"`
	UnsafeMutation         bool              `json:"unsafe_mutation"`
	UnsafeProposal         bool              `json:"unsafe_proposal"`
	Error                  string            `json:"error,omitempty"`
	FailureCategory        string            `json:"failure_category,omitempty"`
	LatencyMS              float64           `json:"latency_ms"`
	Normalization          llm.Normalization `json:"normalization"`
	Baseline               actual            `json:"rules_baseline"`
	BaselineCommandCorrect bool              `json:"baseline_command_correct"`
	BaselineFullCorrect    bool              `json:"baseline_full_correct"`
	Scores                 map[string]bool   `json:"scores"`
	SemanticScored         bool              `json:"semantic_scored"`
}

type report struct {
	PlannedTotal      int                          `json:"planned_total"`
	AbortedReason     string                       `json:"aborted_reason,omitempty"`
	Mode              string                       `json:"mode"`
	Metrics           map[string]metric            `json:"metrics"`
	CoreMetrics       map[string]metric            `json:"core_metrics"`
	BonusMetrics      map[string]metric            `json:"bonus_metrics"`
	FailureCategories map[string]int               `json:"failure_categories"`
	CacheHits         int                          `json:"cache_hits"`
	ScenarioMetrics   map[string]map[string]metric `json:"scenario_metrics"`
	GeneratedAt       string                       `json:"generated_at"`
	Model             string                       `json:"model"`
	BaseURL           string                       `json:"base_url"`
	Temperature       int                          `json:"temperature"`
	Now               string                       `json:"now"`
	Total             int                          `json:"total"`
	CommandAccuracy   struct {
		Correct int     `json:"correct"`
		Total   int     `json:"total"`
		Rate    float64 `json:"rate"`
	} `json:"command_accuracy"`
	FullParseAccuracy struct {
		Correct int     `json:"correct"`
		Total   int     `json:"total"`
		Rate    float64 `json:"rate"`
	} `json:"full_parse_accuracy"`
	UnknownCount        int `json:"unknown_count"`
	UnsafeMutations     int `json:"unsafe_mutations"`
	ModelErrors         int `json:"model_errors"`
	FalseUnknownCount   int `json:"false_unknown_count"`
	UnsafeProposals     int `json:"unsafe_proposals"`
	RulesCommandCorrect int `json:"rules_command_correct"`
	RulesFullCorrect    int `json:"rules_full_correct"`
	LatencyMS           struct {
		Cold float64 `json:"cold"`
		Min  float64 `json:"min"`
		P50  float64 `json:"p50"`
		P95  float64 `json:"p95"`
		Max  float64 `json:"max"`
		Warm struct {
			P50 float64 `json:"p50"`
			P95 float64 `json:"p95"`
		} `json:"warm"`
	} `json:"latency_ms"`
	Failures []result `json:"failures,omitempty"`
	Results  []result `json:"results"`
}

type metric struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Rate    float64 `json:"rate"`
}

func main() {
	input := flag.String("input", "testdata/semantic/semantic_corpus.jsonl", "JSONL corpus")
	output := flag.String("output", "", "JSON report path; stdout when empty")
	baseURL := flag.String("url", "", "optional provider URL")
	modelName := flag.String("model", "", "model identifier returned by API")
	providerName := flag.String("provider", "gigachat", "gigachat or ollama")
	modelsOnly := flag.Bool("models", false, "authenticate and list accessible model IDs")
	nowText := flag.String("now", "2026-10-15T12:30:17+03:00", "fixed evaluation time")
	timezone := flag.String("timezone", "Europe/Moscow", "evaluation timezone")
	timeout := flag.Duration("timeout", 0, "optional per-request timeout override")
	limit := flag.Int("limit", 0, "optional case limit")
	start := flag.Int("start", 0, "optional zero-based case offset")
	mode := flag.String("mode", "full", "model (model only), go (cache only), full (model+Go)")
	cache := flag.String("cache", "artifacts/cache/semantic", "date-independent raw model cache")
	cacheOnly := flag.Bool("cache-only", false, "use saved model outputs only; never call Ollama")
	bypassCache := flag.Bool("bypass-cache", false, "latency benchmark only; do not use for corpus regression")
	contract := flag.String("export-contract", "", "write schema and prompt without inference")
	flag.Parse()
	if *contract != "" {
		raw, _ := json.MarshalIndent(map[string]any{"schema": nlp.SemanticSchema(), "system_prompt": llm.SemanticPrompt()}, "", "  ")
		if err := os.WriteFile(*contract, raw, 0600); err != nil {
			fatalf("%v", err)
		}
		return
	}
	if *mode != "model" && *mode != "go" && *mode != "full" {
		fatalf("invalid mode")
	}
	if *bypassCache && (*cacheOnly || *mode == "go") {
		fatalf("cache bypass cannot be combined with cache-only evaluation")
	}
	if *providerName == "gigachat" && !*cacheOnly && *mode != "go" && os.Getenv("GIGACHAT_INTEGRATION") != "1" {
		fatalf("live GigaChat requires GIGACHAT_INTEGRATION=1")
	}
	cfg, err := config.Load()
	if err != nil {
		fatalf("configuration: %v", err)
	}
	var provider llm.SemanticProvider
	switch *providerName {
	case "gigachat":
		if *modelName != "" {
			cfg.GigaChat.Model = *modelName
		}
		if *baseURL != "" {
			cfg.GigaChat.BaseURL = *baseURL
		}
		if *timeout != 0 {
			cfg.GigaChat.Timeout = *timeout
		}
		giga, err := llm.NewGigaChat(cfg.GigaChat)
		if err != nil {
			fatalf("GigaChat: %v", err)
		}
		defer giga.Close()
		provider = giga
		*baseURL, *modelName = cfg.GigaChat.BaseURL, cfg.GigaChat.Model
		if *modelsOnly {
			ids, err := giga.Models(context.Background())
			if err != nil {
				fatalf("models: %v", err)
			}
			raw, _ := json.Marshal(ids)
			fmt.Println(string(raw))
			return
		}
	case "ollama":
		if *baseURL == "" {
			*baseURL = cfg.OllamaURL
		}
		if *modelName == "" {
			*modelName = cfg.OllamaModel
		}
		if *timeout == 0 {
			*timeout = cfg.OllamaTimeout
		}
		provider, err = llm.NewOllamaSemantic(*baseURL, *modelName, *timeout)
		if err != nil {
			fatalf("Ollama: %v", err)
		}
	default:
		fatalf("invalid provider")
	}

	loc, err := time.LoadLocation(*timezone)
	if err != nil {
		fatalf("invalid -timezone: %v", err)
	}
	parsedNow, err := time.Parse(time.RFC3339, *nowText)
	if err != nil {
		fatalf("invalid -now: %v", err)
	}
	now := parsedNow.In(loc)
	cases, err := readCorpus(*input)
	if err != nil {
		fatalf("read corpus: %v", err)
	}
	if *limit > 0 && *limit < len(cases) {
		if *start < 0 || *start >= len(cases) {
			fatalf("start is outside corpus")
		}
		end := *start + *limit
		if end > len(cases) {
			end = len(cases)
		}
		cases = cases[*start:end]
	} else if *start > 0 {
		if *start >= len(cases) {
			fatalf("start is outside corpus")
		}
		cases = cases[*start:]
	}

	adapter := &llm.Semantic{Provider: provider, CacheDir: *cache, CacheOnly: *cacheOnly || *mode == "go", BypassCache: *bypassCache}
	plannedTotal, consecutiveTransport := len(cases), 0
	abortedReason := ""
	var checkpoint *os.File
	if *output != "" {
		checkpoint, err = os.OpenFile(*output+".progress.jsonl", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fatalf("cannot open evaluation checkpoint")
		}
		defer checkpoint.Close()
	}
	results := make([]result, 0, len(cases))
	latencies := make([]float64, 0, len(cases))
	for _, tc := range cases {
		started := time.Now()
		ctx := context.Background()
		if tc.Source == "voice" || contains(tc.Tags, "voice") || contains(tc.Tags, "asr") {
			ctx = llm.WithSource(ctx, "voice")
		}
		var norm llm.Normalization
		var callErr error
		if *mode == "model" {
			norm.ModelOutput, callErr = adapter.Extract(ctx, tc.Text)
			norm.Status = norm.Semantic.Status
			// Model-only evaluation never invokes mapping or an executor.
		} else {
			norm, callErr = adapter.Normalize(ctx, tc.Text, now)
		}
		cmd := norm.Command
		latency := float64(time.Since(started).Microseconds()) / 1000
		got := toActual(cmd)
		if callErr != nil {
			got.Error = callErr.Error()
		}
		commandCorrect := sameCommand(got, tc.Expected, false)
		fullCorrect := callErr == nil && sameCommand(got, tc.Expected, true)
		unknown := callErr != nil || norm.Status == "unknown"
		unsafe := unsafeMutation(got, tc.Expected)
		baseline := actual{}
		if *mode != "model" {
			baseline = toActual(nlp.Parse(tc.Text, now))
		}
		scores := fieldScores(got, norm.Semantic, tc, callErr)
		if *mode == "model" {
			commandCorrect = scores["action_accuracy"] && scores["kind_accuracy"]
			fullCorrect = tc.SemanticExpected != nil && scores["full_semantic_accuracy"] && callErr == nil
		}
		proposal := toActual(domain.Command{Action: domain.ActionHelp, Duration: time.Hour})
		if *mode != "model" && norm.Semantic.Status == "ok" {
			mapped, mapErr := nlp.MapSemantic(norm.Semantic, tc.Text, now)
			if mapErr == nil {
				proposal = toActual(mapped)
			}
		}
		results = append(results, result{
			ID: tc.ID, Text: tc.Text, Source: tc.Source, Tags: tc.Tags, Expected: tc.Expected, Actual: got,
			CommandCorrect: commandCorrect, FullCorrect: fullCorrect, Unknown: unknown, UnsafeMutation: unsafe,
			Error: errorText(callErr), LatencyMS: latency,
			Normalization: norm, Baseline: baseline,
			BaselineCommandCorrect: sameCommand(baseline, tc.Expected, false), BaselineFullCorrect: sameCommand(baseline, tc.Expected, true),
			Scores: scores, SemanticScored: tc.SemanticExpected != nil,
			UnsafeProposal: unsafeMutation(proposal, tc.Expected),
		})
		if !norm.CacheHit {
			latencies = append(latencies, latency)
		}
		if checkpoint != nil {
			if err := json.NewEncoder(checkpoint).Encode(results[len(results)-1]); err != nil {
				fatalf("cannot write evaluation checkpoint")
			}
		}
		if callErr != nil && norm.Raw == "" {
			consecutiveTransport++
		} else {
			consecutiveTransport = 0
		}
		if len(results)%8 == 0 {
			fmt.Fprintf(os.Stderr, "progress model=%s completed=%d/%d cache=%t last_error=%s\n", *modelName, len(results), plannedTotal, norm.CacheHit, errorText(callErr))
		}
		if consecutiveTransport >= 3 || callErr != nil && (strings.Contains(callErr.Error(), "HTTP 402") || strings.Contains(callErr.Error(), "HTTP 403") || strings.Contains(callErr.Error(), "HTTP 404")) {
			abortedReason = "provider unavailable; stopped bounded screening after consecutive failures"
			break
		}
	}

	var out report
	out.Mode = *mode
	out.PlannedTotal, out.AbortedReason = plannedTotal, abortedReason
	out.Metrics, out.CoreMetrics, out.BonusMetrics = map[string]metric{}, map[string]metric{}, map[string]metric{}
	out.ScenarioMetrics = map[string]map[string]metric{}
	out.FailureCategories = map[string]int{}
	out.GeneratedAt = time.Now().Format(time.RFC3339)
	out.Model, out.BaseURL, out.Temperature, out.Now = *modelName, *baseURL, 0, *nowText
	out.Total, out.Results = len(results), results
	for index, r := range results {
		if r.Normalization.CacheHit {
			out.CacheHits++
		}
		for name, pass := range r.Scores {
			if *mode == "model" && (name == "domain_command_accuracy" || name == "accepted_target_accuracy" || name == "accepted_action_accuracy" || name == "temporal_accuracy") {
				continue
			}
			if !scoreApplies(name, r.Expected, r.SemanticScored) {
				continue
			}
			addMetric(out.Metrics, name, pass)
			group := out.BonusMetrics
			if r.Expected.Action == "create" && r.Expected.Recurrence == nil || r.Expected.Action == "list" || r.Expected.Action == "search" {
				group = out.CoreMetrics
			}
			addMetric(group, name, pass)
			scenario := scenarioFor(r.Expected)
			if out.ScenarioMetrics[scenario] == nil {
				out.ScenarioMetrics[scenario] = map[string]metric{}
			}
			addMetric(out.ScenarioMetrics[scenario], name, pass)
		}
		if !r.FullCorrect && *mode != "model" {
			category := "other"
			for _, name := range []string{"action_accuracy", "kind_accuracy", "title_accuracy", "query_accuracy", "target_accuracy", "changes_accuracy", "temporal_accuracy"} {
				if scoreApplies(name, r.Expected, r.SemanticScored) && !r.Scores[name] {
					category = name
					break
				}
			}
			if r.Error != "" {
				category = "validation_or_model_error"
			}
			out.FailureCategories[category]++
			r.FailureCategory = category
			out.Results[index].FailureCategory = category
		}
		if r.CommandCorrect {
			out.CommandAccuracy.Correct++
		}
		if r.FullCorrect {
			out.FullParseAccuracy.Correct++
		}
		if r.Unknown {
			out.UnknownCount++
			if r.Expected.Action != "help" {
				out.FalseUnknownCount++
			}
		}
		if r.UnsafeMutation {
			out.UnsafeMutations++
		}
		if r.Error != "" {
			out.ModelErrors++
		}
		if r.Normalization.UnsafeProposal || r.UnsafeProposal {
			out.UnsafeProposals++
		}
		if r.BaselineCommandCorrect {
			out.RulesCommandCorrect++
		}
		if r.BaselineFullCorrect {
			out.RulesFullCorrect++
		}
		if len(out.Failures) < 20 && (!r.FullCorrect || r.Unknown || r.UnsafeMutation) {
			out.Failures = append(out.Failures, r)
		}
	}
	out.CommandAccuracy.Total = out.Total
	out.CommandAccuracy.Rate = rate(out.CommandAccuracy.Correct, out.Total)
	out.FullParseAccuracy.Total = out.Total
	if *mode == "model" {
		out.FullParseAccuracy.Total = 0
		for _, r := range results {
			if r.SemanticScored {
				out.FullParseAccuracy.Total++
			}
		}
	}
	out.FullParseAccuracy.Rate = rate(out.FullParseAccuracy.Correct, out.FullParseAccuracy.Total)
	if len(latencies) > 0 {
		out.LatencyMS.Cold = latencies[0]
		warm := append([]float64(nil), latencies[1:]...)
		sort.Float64s(latencies)
		sort.Float64s(warm)
		out.LatencyMS.Min = latencies[0]
		out.LatencyMS.P50 = percentile(latencies, .50)
		out.LatencyMS.P95 = percentile(latencies, .95)
		out.LatencyMS.Max = latencies[len(latencies)-1]
		out.LatencyMS.Warm.P50 = percentile(warm, .50)
		out.LatencyMS.Warm.P95 = percentile(warm, .95)
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	if *output == "" {
		fmt.Println(string(encoded))
		return
	}
	if err := os.WriteFile(*output, append(encoded, '\n'), 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	summary, _ := json.Marshal(map[string]any{
		"mode": out.Mode, "total": out.Total, "passed": out.FullParseAccuracy.Correct,
		"scored": out.FullParseAccuracy.Total, "unscored": out.Total - out.FullParseAccuracy.Total,
		"failed": out.FullParseAccuracy.Total - out.FullParseAccuracy.Correct, "action_accuracy": out.Metrics["action_accuracy"],
		"full_parse_accuracy": out.FullParseAccuracy, "unsafe_mutations": out.UnsafeMutations,
		"unknown": out.UnknownCount, "cache_hits": out.CacheHits, "latency_ms": out.LatencyMS,
		"failure_categories": out.FailureCategories,
	})
	fmt.Println(string(summary))
	fmt.Fprintf(os.Stderr, "wrote %s\n", *output)
}

func readCorpus(path string) ([]corpusCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []corpusCase
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var tc corpusCase
		if err := json.Unmarshal([]byte(line), &tc); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		cases = append(cases, tc)
	}
	return cases, nil
}

func toActual(cmd domain.Command) actual {
	// Compare fields used by the existing executor. Creation/move period
	// metadata duplicates an explicit date; list ignores date/time/title/query.
	// Applying the same projection to rules and semantic output is essential
	// for a fair baseline, not a change to either parser's result.
	if (cmd.Action == domain.ActionCreate || cmd.Action == domain.ActionUpdate) && cmd.Date != nil {
		cmd.PeriodStart, cmd.PeriodEnd = nil, nil
	}
	if cmd.Action == domain.ActionList {
		cmd.Date, cmd.Time, cmd.Title, cmd.Query = nil, nil, "", ""
	}
	if cmd.Action == domain.ActionSearch {
		cmd.Date, cmd.Time, cmd.Title = nil, nil, ""
	}
	var recurrence map[string]any
	if cmd.Recurrence != nil {
		raw, _ := json.Marshal(cmd.Recurrence)
		_ = json.Unmarshal(raw, &recurrence)
	}
	return actual{
		Action: cmd.Action.String(), Kind: kindString(cmd.Kind), Title: cmd.Title, Query: cmd.Query,
		Date: dateString(cmd.Date), Time: timeString(cmd.Time), PeriodStart: dateString(cmd.PeriodStart), PeriodEnd: dateString(cmd.PeriodEnd),
		ReminderMinutes: cmd.ReminderMinutes, ReminderEnabled: cmd.ReminderEnabled, DurationMinutes: int(cmd.Duration / time.Minute),
		TargetID: cmd.TargetID, StartSecond: cmd.StartSecond, Provider: cmd.Provider, Error: cmd.Error,
		Recurrence: recurrence, Selector: cmd.Selector,
	}
}

func kindString(kind domain.EventType) *string {
	if kind == domain.EventUnknown {
		return nil
	}
	value := kind.String()
	return &value
}

func dateString(value *domain.Date) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func timeString(value *domain.TimeOfDay) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func sameCommand(got actual, want expected, full bool) bool {
	if want.Action == "help" && got.Action == "help" {
		return true
	}
	if got.Action != want.Action || !samePtr(got.Kind, want.Kind) {
		return false
	}
	if !full {
		return true
	}
	if ((got.Error != "") != want.ErrorExpected && want.Action != "help") || !containsFields(got.Recurrence, want.Recurrence) {
		return false
	}
	return normalize(got.Title) == normalize(want.Title) &&
		((got.Action == "update" || got.Action == "delete" || got.Action == "complete") && normalizeTarget(got.Query) == normalizeTarget(want.Query) || normalize(got.Query) == normalize(want.Query)) &&
		samePtr(got.Date, want.Date) &&
		samePtr(got.Time, want.Time) &&
		samePtr(got.PeriodStart, want.PeriodStart) &&
		samePtr(got.PeriodEnd, want.PeriodEnd) &&
		sameInt(got.ReminderMinutes, want.ReminderMinutes) &&
		sameBool(got.ReminderEnabled, want.ReminderEnabled) &&
		got.DurationMinutes == want.DurationMinutes &&
		sameInt64(got.TargetID, want.TargetID) &&
		got.StartSecond == want.StartSecond
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func addMetric(metrics map[string]metric, name string, pass bool) {
	m := metrics[name]
	m.Total++
	if pass {
		m.Correct++
	}
	m.Rate = rate(m.Correct, m.Total)
	metrics[name] = m
}

func scoreApplies(name string, want expected, semanticScored bool) bool {
	switch name {
	case "full_semantic_accuracy", "temporal_extraction_accuracy":
		return semanticScored
	case "title_accuracy":
		return want.Title != ""
	case "query_accuracy":
		return want.Action == "search" && want.Query != ""
	case "target_accuracy", "accepted_target_accuracy":
		return want.Action == "update" || want.Action == "delete" || want.Action == "complete"
	case "changes_accuracy":
		return semanticScored && want.Action == "update"
	default:
		return true
	}
}

func fieldScores(got actual, semantic nlp.SemanticCommand, tc corpusCase, err error) map[string]bool {
	want := tc.Expected
	semanticAction := ""
	if semantic.Action != nil {
		semanticAction = *semantic.Action
	}
	if semantic.Status == "unknown" || semantic.Status == "ambiguous" {
		semanticAction = "help"
	}
	action := semanticAction == want.Action
	title, query, targetQuery := "", "", ""
	if semantic.Title != nil {
		title = *semantic.Title
	}
	if semantic.Query != nil {
		query = *semantic.Query
	}
	if semantic.Changes != nil && semantic.Changes.Title != nil {
		title = *semantic.Changes.Title
	}
	if semantic.Target != nil && semantic.Target.Query != nil {
		targetQuery = *semantic.Target.Query
	}
	target := normalizeTarget(got.Query) == normalizeTarget(want.Query) && sameInt64(got.TargetID, want.TargetID)
	if want.Selector != nil {
		a, _ := json.Marshal(got.Selector)
		b, _ := json.Marshal(want.Selector)
		target = target && string(a) == string(b)
	}
	temporal := samePtr(got.Date, want.Date) && samePtr(got.Time, want.Time) && samePtr(got.PeriodStart, want.PeriodStart) && samePtr(got.PeriodEnd, want.PeriodEnd) && got.StartSecond == want.StartSecond && containsFields(got.Recurrence, want.Recurrence)
	fullSemantic := false
	temporalSemantic := false
	semanticTarget := normalizeTarget(targetQuery) == normalizeTarget(want.Query)
	// Absolute legacy domain gold cannot score source-language change expressions.
	changesCorrect := false
	if tc.SemanticExpected != nil {
		// Compare the actual structured boundary before Go mapping. No clock or
		// executor projection can hide a semantic extraction mistake.
		a := semanticProjection(semantic)
		b := semanticProjection(*tc.SemanticExpected)
		fullSemantic = normalize(string(a)) == normalize(string(b))
		temporalSemantic = normalize(string(semanticTemporalProjection(semantic))) == normalize(string(semanticTemporalProjection(*tc.SemanticExpected)))
		a = semanticProjection(nlp.SemanticCommand{Target: semantic.Target})
		b = semanticProjection(nlp.SemanticCommand{Target: tc.SemanticExpected.Target})
		semanticTarget = normalize(string(a)) == normalize(string(b))
		a = semanticProjection(nlp.SemanticCommand{Changes: semantic.Changes})
		b = semanticProjection(nlp.SemanticCommand{Changes: tc.SemanticExpected.Changes})
		changesCorrect = normalize(string(a)) == normalize(string(b))
	}
	return map[string]bool{
		"action_accuracy":              action,
		"accepted_action_accuracy":     got.Action == want.Action && err == nil,
		"kind_accuracy":                samePtr(semantic.Kind, want.Kind),
		"title_accuracy":               normalize(title) == normalize(want.Title),
		"query_accuracy":               normalize(query) == normalize(want.Query),
		"target_accuracy":              semanticTarget,
		"accepted_target_accuracy":     target,
		"changes_accuracy":             changesCorrect,
		"temporal_accuracy":            temporal,
		"temporal_extraction_accuracy": temporalSemantic,
		"full_semantic_accuracy":       fullSemantic,
		"domain_command_accuracy":      sameCommand(got, want, true) && target && err == nil,
	}
}

func semanticTemporalProjection(s nlp.SemanticCommand) []byte {
	s.Status, s.Action, s.Kind, s.Title, s.Query, s.Clarification = "", nil, nil, nil, nil, nil
	if s.Target != nil {
		target := *s.Target
		target.Query, target.All = nil, false
		s.Target = &target
		if target.DateExpression == nil && target.TimeExpression == nil && target.PeriodExpression == nil {
			s.Target = nil
		}
	}
	if s.Changes != nil {
		changes := *s.Changes
		changes.Title = nil
		s.Changes = &changes
		if changes.DateExpression == nil && changes.TimeExpression == nil && changes.RelativeTimeExpression == nil {
			s.Changes = nil
		}
	}
	return semanticProjection(s)
}

func normalizeTarget(text string) string {
	text = normalize(text)
	for _, prefix := range []string{"встреча ", "встречу ", "задача ", "задачу ", "напоминание "} {
		text = strings.TrimPrefix(text, prefix)
	}
	for _, prefix := range []string{"с ", "со ", "про ", "по ", "о ", "об "} {
		text = strings.TrimPrefix(text, prefix)
	}
	return strings.TrimSpace(text)
}

func semanticProjection(s nlp.SemanticCommand) []byte {
	raw, _ := json.Marshal(s)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	delete(obj, "clarification") // wording of a clarification is not language accuracy
	var clean func(map[string]any)
	clean = func(m map[string]any) {
		for key, value := range m {
			if child, ok := value.(map[string]any); ok {
				clean(child)
			}
			if text, ok := value.(string); ok && strings.HasSuffix(key, "_expression") {
				text = normalize(text)
				for _, prefix := range []string{"в ", "во ", "на ", "к "} {
					text = strings.TrimPrefix(text, prefix)
				}
				for _, word := range []string{"завтрашнюю", "завтрашней", "завтрашний"} {
					text = strings.ReplaceAll(text, word, "завтра")
				}
				text = strings.NewReplacer("пятницу", "пятница", "субботу", "суббота", "среду", "среда").Replace(text)
				m[key] = text
			}
		}
	}
	clean(obj)
	// "сегодня" preceding an elapsed interval is redundant, not a model date
	// calculation. Compare the extracted interval, whose exact instant Go owns.
	if obj["relative_time_expression"] != nil && obj["date_expression"] == "сегодня" {
		delete(obj, "date_expression")
	}
	raw, _ = json.Marshal(obj)
	return raw
}

func scenarioFor(w expected) string {
	if w.Recurrence != nil {
		return "I_recurrence"
	}
	switch w.Action {
	case "create":
		if w.Kind != nil && *w.Kind == "meeting" {
			return "B_create_meeting"
		}
		return "A_create_task_reminder"
	case "list":
		return "C_list"
	case "search":
		return "D_search"
	case "update":
		return "E_update"
	case "delete":
		return "F_delete"
	case "complete":
		return "G_complete"
	default:
		return "J_ambiguity_unknown"
	}
}

func containsFields(got, want map[string]any) bool {
	if want == nil {
		return got == nil
	}
	for key, expected := range want {
		if key == "time" {
			key = "local_time"
		} // legacy gold alias; domain.Recurrence uses local_time
		a, _ := json.Marshal(got[key])
		b, _ := json.Marshal(expected)
		if string(a) != string(b) {
			return false
		}
	}
	return true
}

func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return normalize(*a) == normalize(*b)
}
func sameInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func sameBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func sameInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func normalize(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(text)), "ё", "е")), " ")
}

func unsafeMutation(got actual, want expected) bool {
	if got.Error != "" {
		return false
	} // executor clarifies; it cannot mutate an event
	mutating := map[string]bool{"create": true, "update": true, "complete": true, "delete": true, "restore": true}
	nonMutating := map[string]bool{"help": true, "list": true, "search": true}
	if nonMutating[want.Action] && mutating[got.Action] {
		return true
	}
	if want.TargetID == nil && got.TargetID != nil {
		return true
	}
	if got.Action == "update" || got.Action == "delete" || got.Action == "complete" {
		if got.Action != want.Action || !sameInt64(got.TargetID, want.TargetID) || !samePtr(got.Kind, want.Kind) || normalizeTarget(got.Query) != normalizeTarget(want.Query) {
			return true
		}
		if want.Selector != nil {
			a, _ := json.Marshal(got.Selector)
			b, _ := json.Marshal(want.Selector)
			if string(a) != string(b) {
				return true
			}
		}
	}
	return (want.Action == "help" || want.Action == "list" || want.Action == "search") && mutating[got.Action]
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values))*p+.999999) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}

func rate(correct, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(correct) / float64(total)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
