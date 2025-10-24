package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/MalithGihan/uigp-service/internal/fusion"
	"github.com/MalithGihan/uigp-service/internal/ingest"
	"github.com/MalithGihan/uigp-service/internal/store"
	"github.com/MalithGihan/uigp-service/internal/validate"
	"github.com/MalithGihan/uigp-service/pkg/types"
)

type pingResp struct {
	OK              bool   `json:"ok"`
	OllamaURL       string `json:"ollama_url"`
	OllamaReachable bool   `json:"ollama_reachable"`
	Note            string `json:"note,omitempty"`
}

func loadIG(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}
func saveIG(path string, ig any) { _ = os.WriteFile(path, mustJSONBytes(ig), 0o644) }

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mustJSONBytes(v any) []byte { b, _ := json.Marshal(v); return b }

func main() {
	_ = godotenv.Load()
	port := getenv("PORT", "8081")
	model := getenv("OLLAMA_MODEL", "llama3:instruct")
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
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(ollamaURL + "/api/tags")
		reachable := err == nil && resp != nil && resp.StatusCode < 500
		if resp != nil {
			resp.Body.Close()
		}

		out := pingResp{OK: true, OllamaURL: ollamaURL, OllamaReachable: reachable}
		if !reachable {
			out.Note = "Ollama not reachable — is the desktop app/server running?"
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

	// Get Intermediate Graph
	r.Get("/jobs/{id}/intermediate", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		jobDir := st.JobDir(id)
		upDir := filepath.Join(jobDir, "uploads")
		igPath := filepath.Join(jobDir, "intermediate.json")
		refresh := r.URL.Query().Get("refresh") == "true"

		// Use cache if present and not refreshing
		if !refresh {
			if igm, err := loadIG(igPath); err == nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(igm)
				return
			}
		}

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
		saveIG(igPath, ig)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ig)
	})

	// Fuse (mock)
	r.Post("/jobs/{id}/fuse", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		jobDir := st.JobDir(id)
		igPath := filepath.Join(jobDir, "intermediate.json")

		var ig types.IntermediateGraph
		if igm, err := loadIG(igPath); err == nil {
			// reuse cache
			b, _ := json.Marshal(igm)
			_ = json.Unmarshal(b, &ig)
		} else {
			// build from uploads
			upDir := filepath.Join(jobDir, "uploads")
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
			ig = ingest.BuildIntermediate(parsed)
			saveIG(igPath, ig)
		}

		chat := fusion.LoadChat(jobDir)

		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		out, err := fusion.FuseWithOllama(ctx, ollamaURL, model, ig, chat)
		if err != nil || out == nil {
			log.Printf("FuseWithOllama fallback: %v", err)
			out = fusion.MockFromIntermediate(ig)
			out["__note"] = "LLM unavailable, returned mock spec"
		}

		out = fusion.Sanitize(out)
		if err := validate.ValidateMap(out); err != nil {
			rctx, cancel2 := context.WithTimeout(r.Context(), 60*time.Second)
			defer cancel2()
			if repaired, rerr := fusion.RepairWithOllama(rctx, ollamaURL, model, out, err.Error()); rerr == nil && repaired != nil && validate.ValidateMap(repaired) == nil {
				out = repaired
			} else {
				if _, ok := out["metadata"]; !ok {
					out["metadata"] = map[string]any{"schemaVersion": "0.1.0", "generator": "repair-fallback"}
				}
			}
		}

		_ = os.WriteFile(filepath.Join(jobDir, "last_spec.json"), mustJSONBytes(out), 0o644)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	r.Get("/jobs/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}

		jobDir := st.JobDir(id)
		specPath := filepath.Join(jobDir, "last_spec.json")
		b, err := os.ReadFile(specPath)
		if err != nil {
			http.Error(w, "spec not found - run /jobs/{id}/fuse first", http.StatusNotFound)
			return
		}
		var spec map[string]any
		_ = json.Unmarshal(b, &spec)

		// ensure exports dir
		expDir := filepath.Join(jobDir, "exports")
		_ = os.MkdirAll(expDir, 0o755)

		switch format {
		case "yaml", "yml":
			data, _ := yaml.Marshal(spec)
			out := filepath.Join(expDir, "architecture.yaml")
			_ = os.WriteFile(out, data, 0o644)

			w.Header().Set("Content-Type", "application/x-yaml")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="architecture-%s.yaml"`, id))
			w.WriteHeader(http.StatusOK)
			w.Write(data)
		default:
			data, _ := json.MarshalIndent(spec, "", "  ")
			out := filepath.Join(expDir, "architecture.json")
			_ = os.WriteFile(out, data, 0o644)

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="architecture-%s.json"`, id))
			w.WriteHeader(http.StatusOK)
			w.Write(data)
		}
	})

	r.Get("/jobs/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		jobDir := st.JobDir(id)
		specPath := filepath.Join(jobDir, "last_spec.json")
		download := r.URL.Query().Get("download") == "true"

		b, err := os.ReadFile(specPath)
		if err != nil {
			http.Error(w, "spec not found - run /fuse first", 404)
			return
		}

		var spec map[string]any
		_ = json.Unmarshal(b, &spec)

		getArr := func(k string) []any {
			if v, ok := spec[k].([]any); ok {
				return v
			}
			return []any{}
		}
		out := map[string]any{
			"ok": true,
			"counts": map[string]any{
				"services":     len(getArr("services")),
				"dependencies": len(getArr("dependencies")),
				"datastores":   len(getArr("datastores")),
				"topics":       len(getArr("topics")),
				"gaps":         len(getArr("gaps")),
				"conflicts":    len(getArr("conflicts")),
			},
			"gaps":      getArr("gaps"),
			"conflicts": getArr("conflicts"),
		}

		// save on server
		expDir := filepath.Join(jobDir, "exports")
		_ = os.MkdirAll(expDir, 0o755)
		repPath := filepath.Join(expDir, "report-"+id+".json")
		_ = os.WriteFile(repPath, mustJSONBytes(out), 0o644)

		if download {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="report-`+id+`.json"`)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	log.Printf("uigp-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
