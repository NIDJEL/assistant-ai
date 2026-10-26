// retrieval-eval measures read-only retrieval against a frozen, inspectable qrels file.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/evaluation"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

type judgment struct {
	Name, Split, Query, Rationale string
	Relevant                      []int64
}
type corpus struct {
	DatasetSHA256 string
	Queries       []judgment
}
type row struct {
	Name, Split, Mode string
	IDs               []int64
	Recall10, MRR, MS float64
	Warning           string
}
type aggregate struct {
	Count                                  int
	Recall10, MRR, Success10, P50MS, P95MS float64
}
type report struct {
	SearchCalls                                                        int
	Status                                                             string
	EmbeddingErrors                                                    []string
	Source                                                             evaluation.Source
	CreatedAt, SourceSHA, DatasetSHA256, CorpusSHA256, Model, Timezone string
	DatasetRows, EmbeddingCalls                                        int
	Summary                                                            map[string]aggregate
	Results                                                            []row
}
type measuredEmbedder struct {
	search.Embedder
	calls  int
	errors []string
}

func (m *measuredEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	m.calls++
	vectors, err := m.Embedder.Embed(ctx, texts)
	if err != nil {
		m.errors = append(m.errors, err.Error())
	}
	return vectors, err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dataset := flag.String("dataset", "data/voice_assistant_events_1000.csv", "CSV snapshot")
	qrels := flag.String("qrels", "testdata/retrieval/qrels.json", "exhaustive relevance judgments")
	mode := flag.String("mode", "lexical", "lexical, cloud (chat semantic search), or both (live embeddings)")
	output := flag.String("output", "artifacts/retrieval_eval.json", "JSON evidence")
	budget := flag.Duration("budget", 90*time.Second, "embedding preparation budget per query for cold benchmark")
	flag.Parse()
	if *mode != "lexical" && *mode != "both" && *mode != "cloud" {
		return fmt.Errorf("mode must be lexical, cloud or both")
	}
	data, err := os.ReadFile(*dataset)
	if err != nil {
		return err
	}
	labels, err := os.ReadFile(*qrels)
	if err != nil {
		return err
	}
	var c corpus
	if err = json.Unmarshal(labels, &c); err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if digest != c.DatasetSHA256 {
		return fmt.Errorf("dataset SHA differs from frozen judgments")
	}
	file, err := os.Open(*dataset)
	if err != nil {
		return err
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return err
	}
	events := []domain.Event{}
	for _, r := range records[1:] {
		id, err := strconv.ParseInt(r[0], 10, 64)
		if err != nil {
			return err
		}
		events = append(events, domain.Event{ID: id, Title: r[3]})
	}
	engine, err := search.New()
	if err != nil {
		return err
	}
	source := evaluation.Identity()
	r := report{Source: source, CreatedAt: time.Now().UTC().Format(time.RFC3339), SourceSHA: source.Commit, DatasetSHA256: digest, CorpusSHA256: fmt.Sprintf("%x", sha256.Sum256(labels)), DatasetRows: len(events), Timezone: "Europe/Moscow", Summary: map[string]aggregate{}}
	var meter *measuredEmbedder
	if *mode == "both" || *mode == "cloud" {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		g, err := llm.NewGigaChat(cfg.GigaChat)
		if err != nil {
			return err
		}
		defer g.Close()
		if *mode == "cloud" {
			provider := llm.CloudSearch{Client: g}
			engine.EnableCloud(provider, *budget)
			r.Model = provider.ModelID()
		} else {
			meter = &measuredEmbedder{Embedder: llm.GigaEmbeddings{Client: g, Model: cfg.EmbeddingModel}}
			engine.EnableHybrid(meter, *budget)
			r.Model = meter.ModelID()
		}
	}
	ctx := repository.WithUser(context.Background(), "disposable-evaluation-only")
	for _, q := range c.Queries {
		if len(q.Relevant) == 0 {
			return fmt.Errorf("query %s has no judgments", q.Name)
		}
		modes := []string{"lexical"}
		if meter != nil {
			modes = append(modes, "hybrid")
		}
		if *mode == "cloud" {
			modes = append(modes, "cloud")
		}
		for _, method := range modes {
			started := time.Now()
			var result []domain.Event
			warning := ""
			if method == "lexical" {
				result = engine.Search(events, q.Query, 10)
			} else {
				prepared := engine.Prepare(ctx, events, q.Query)
				var status *domain.SearchStatus
				result, status = engine.SearchContext(prepared, events, q.Query, 10)
				warning = status.Warning
				r.SearchCalls += status.Calls
				if status.Mode != method {
					warning = "fallback: " + warning
				}
			}
			item := row{Name: q.Name, Split: q.Split, Mode: method, MS: float64(time.Since(started).Microseconds()) / 1000, Warning: warning}
			matched := 0
			for i, e := range result {
				item.IDs = append(item.IDs, e.ID)
				for _, id := range q.Relevant {
					if id == e.ID {
						matched++
						if item.MRR == 0 {
							item.MRR = 1 / float64(i+1)
						}
						break
					}
				}
			}
			item.Recall10 = float64(matched) / float64(len(q.Relevant))
			r.Results = append(r.Results, item)
		}
	}
	splits := map[string]bool{}
	for _, item := range r.Results {
		splits[item.Split] = true
	}
	for split := range splits {
		for _, method := range []string{"lexical", "hybrid", "cloud"} {
			a := aggregate{}
			latency := []float64{}
			for _, row := range r.Results {
				if row.Split == split && row.Mode == method {
					a.Count++
					a.Recall10 += row.Recall10
					a.MRR += row.MRR
					if row.MRR > 0 {
						a.Success10++
					}
					latency = append(latency, row.MS)
				}
			}
			if a.Count > 0 {
				sort.Float64s(latency)
				a.Recall10 /= float64(a.Count)
				a.MRR /= float64(a.Count)
				a.Success10 /= float64(a.Count)
				a.P50MS = latency[(len(latency)-1)/2]
				a.P95MS = latency[(len(latency)*95+99)/100-1]
				r.Summary[split+"/"+method] = a
			}
		}
	}
	if meter != nil {
		r.EmbeddingCalls = meter.calls
		r.EmbeddingErrors = meter.errors
	}
	r.Status = "complete"
	if len(r.EmbeddingErrors) > 0 {
		r.Status = "degraded"
	}
	for _, row := range r.Results {
		if row.Warning != "" {
			r.Status = "degraded"
		}
	}
	encoded, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*output), 0750); err != nil {
		return err
	}
	if err = os.WriteFile(*output, encoded, 0600); err != nil {
		return err
	}
	for key, value := range r.Summary {
		fmt.Printf("%s n=%d MRR=%.3f Recall@10=%.3f Success@10=%.3f p50=%.0fms p95=%.0fms\n", key, value.Count, value.MRR, value.Recall10, value.Success10, value.P50MS, value.P95MS)
	}
	if r.Status == "degraded" {
		return fmt.Errorf("live retrieval evaluation incomplete: provider failure/fallback; inspect saved report")
	}
	return nil
}
