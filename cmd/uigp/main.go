package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/MalithGihan/uigp-service/internal/fusion"
	"github.com/MalithGihan/uigp-service/internal/ingest"
	"github.com/MalithGihan/uigp-service/internal/store"
)

type pingResp struct {
	OK              bool   `json:"ok"`
	OllamaURL       string `json:"ollama_url"`
	OllamaReachable bool   `json:"ollama_reachable"`
	Note            string `json:"note,omitempty"`
}

func main() {
	_ = godotenv.Load()
	port := getenv("PORT", "8081")
	ollamaURL := getenv("OLLAMA_URL", "http://localhost:11434")

	dataRoot := getenv("DATA_ROOT", "./projects")
	st, err := store.New(dataRoot)
	if err != nil {
		log.Fatal(err)
	}

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"service":"uigp-service"}`))
	})

	r.Get("/llm/ping", func(w http.ResponseWriter, _ *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"model":   "llama3:instruct",
			"format":  "json",
			"system":  `Return ONLY valid JSON: {"ok":true}`,
			"prompt":  "Say nothing else—just the JSON.",
			"options": map[string]any{"temperature": 0.2},
		})
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(ollamaURL+"/api/generate", "application/json", bytes.NewReader(body))
		reachable := err == nil && resp != nil && resp.StatusCode < 500
		if resp != nil {
			resp.Body.Close()
		}
		out := pingResp{OK: true, OllamaURL: ollamaURL, OllamaReachable: reachable}
		if !reachable {
			out.Note = "Ollama not running or model not pulled yet — OK for now."
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// Upload
	r.Post("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		chat := r.FormValue("chat")
		jobID := uuid.NewString()
		if _, err := st.MkJob(jobID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, fh := range r.MultipartForm.File["files"] {
			src, err := fh.Open()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer src.Close()
			dstPath := filepath.Join(st.JobDir(jobID), "uploads", fh.Filename)
			dst, err := os.Create(dstPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if _, err := io.Copy(dst, src); err != nil {
				dst.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			dst.Close()
		}
		_ = os.WriteFile(filepath.Join(st.JobDir(jobID), "chat.txt"), []byte(chat), 0o644)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "jobId": jobID})
	})

	// Parse to IntermediateGraph
	r.Get("/jobs/{id}/intermediate", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		upDir := filepath.Join(st.JobDir(id), "uploads")
		entries, err := os.ReadDir(upDir)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}

		var parsed []ingest.ParsedFile
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fp := filepath.Join(upDir, e.Name())
			switch ingest.DetectType(e.Name()) {
			case "drawio":
				if p, err := ingest.ParseDrawIO(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "puml":
				if p, err := ingest.ParsePUML(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "svg":
				if p, err := ingest.ParseSVG(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "pdf":
				if p, err := ingest.ParsePDF(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "raster":
				if p, err := ingest.ParseRaster(fp); err == nil {
					parsed = append(parsed, p)
				}
			}
		}
		ig := ingest.BuildIntermediate(parsed)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ig)
	})

	// Fuse (mock)
	r.Post("/jobs/{id}/fuse", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		upDir := filepath.Join(st.JobDir(id), "uploads")
		entries, err := os.ReadDir(upDir)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}

		var parsed []ingest.ParsedFile
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			fp := filepath.Join(upDir, e.Name())
			switch ingest.DetectType(e.Name()) {
			case "drawio":
				if p, err := ingest.ParseDrawIO(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "puml":
				if p, err := ingest.ParsePUML(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "svg":
				if p, err := ingest.ParseSVG(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "pdf":
				if p, err := ingest.ParsePDF(fp); err == nil {
					parsed = append(parsed, p)
				}
			case "raster":
				if p, err := ingest.ParseRaster(fp); err == nil {
					parsed = append(parsed, p)
				}
			}
		}
		ig := ingest.BuildIntermediate(parsed)
		spec := fusion.MockFromIntermediate(ig)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)
	})

	log.Printf("uigp-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
