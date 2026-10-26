package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

type evaluationProtocol struct {
	FrozenAt    string            `json:"frozen_at"`
	Input       string            `json:"input"`
	InputSHA256 string            `json:"input_sha256"`
	Hashes      map[string]string `json:"contract_hashes"`
	Files       map[string]string `json:"code_hashes"`
}

func freezeProtocol(input string, content []byte) (evaluationProtocol, error) {
	hash := func(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }
	p := evaluationProtocol{FrozenAt: time.Now().UTC().Format(time.RFC3339), Input: filepath.ToSlash(filepath.Clean(input)), InputSHA256: hash(content), Hashes: map[string]string{}, Files: map[string]string{}}
	for name, value := range map[string]any{"contextual_prompt": llm.ContextualPrompt(), "semantic_prompt": llm.SemanticPrompt(), "contextual_schema": nlp.ContextualSchema(), "semantic_schema": nlp.SemanticSchema()} {
		raw, err := json.Marshal(value)
		if err != nil {
			return p, err
		}
		p.Hashes[name] = hash(raw)
	}
	for _, root := range []string{"internal", "cmd/context-eval", "tests", "scripts/context_corpus_v2.py", "go.mod", "go.sum"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			p.Files[filepath.ToSlash(path)] = hash(raw)
			return nil
		})
		if err != nil {
			return p, err
		}
	}
	return p, nil
}
