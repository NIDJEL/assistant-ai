package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRealisticSmokeExecution(t *testing.T) {
	data, err := os.ReadFile("../../testdata/simple_llm/smoke.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []scenario
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 30 || len(cases) > 40 {
		t.Fatal("keep the smoke corpus small", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r, err := runScenario(t.Context(), c, &recordingInterpreter{})
			if err != nil {
				t.Fatal(err)
			}
			if !r.Pass {
				raw, _ := json.MarshalIndent(r, "", "  ")
				t.Fatal(string(raw))
			}
		})
	}
}

func TestTopTenMandatoryScenarios(t *testing.T) {
	data, err := os.ReadFile("../../testdata/simple_llm/top10.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []scenario
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatal("mandatory scenarios missing")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r, err := runScenario(t.Context(), c, &recordingInterpreter{})
			if err != nil {
				t.Fatal(err)
			}
			if !r.Pass {
				raw, _ := json.MarshalIndent(r, "", "  ")
				t.Fatal(string(raw))
			}
		})
	}
}

func TestScreenshotSeriesScenarios(t *testing.T) {
	data, err := os.ReadFile("../../testdata/simple_llm/series_occurrences.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []scenario
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("series regression scenarios missing")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			r, err := runScenario(t.Context(), c, &recordingInterpreter{})
			if err != nil {
				t.Fatal(err)
			}
			if !r.Pass {
				raw, _ := json.MarshalIndent(r, "", "  ")
				t.Fatal(string(raw))
			}
		})
	}
}
