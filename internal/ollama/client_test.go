package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUnloadSendsKeepAliveZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "model:test" || body["keep_alive"] != float64(0) {
			t.Errorf("request body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(ChatResponse{Done: true})
	}))
	defer server.Close()

	client := New(server.URL+"/", time.Second)
	if err := client.Unload(context.Background(), "model:test"); err != nil {
		t.Fatal(err)
	}
}

func TestRunningModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ps" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"one:latest"},{"model":"two:test"}]}`))
	}))
	defer server.Close()

	client := New(server.URL, time.Second)
	models, err := client.RunningModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "one:latest" || models[1] != "two:test" {
		t.Fatalf("RunningModels() = %v", models)
	}
}
