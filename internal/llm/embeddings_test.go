package llm

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestEmbeddingsAuthenticateReorderAndRejectMalformedVectors(t *testing.T) {
	for index, body := range []string{
		`{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}]}`,
		`{"data":[{"index":0,"embedding":[1,0]},{"index":0,"embedding":[0,1]}]}`,
		`{"data":[{"index":0,"embedding":[1,0]},{"index":1,"embedding":[0]}]}`,
		`{"data":[{"index":0,"embedding":[0,0]},{"index":1,"embedding":[1,0]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth" {
					tokenResponse(w, "embedding-token")
					return
				}
				if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer embedding-token" {
					t.Error("wrong embedding request")
				}
				var p struct {
					Model string
					Input []string
				}
				json.NewDecoder(r.Body).Decode(&p)
				if p.Model != "Embeddings-2" || len(p.Input) != 2 {
					t.Error(p)
				}
				w.Write([]byte(body))
			})
			vectors, err := (GigaEmbeddings{Client: g}).Embed(t.Context(), []string{"один", "два"})
			if index == 0 {
				if err != nil || vectors[0][0] != 1 || vectors[1][1] != 1 {
					t.Fatal(vectors, err)
				}
			} else if err == nil {
				t.Fatal("malformed vector accepted")
			}
		})
	}
}
