package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/app"
	"github.com/Keyhole-Koro/SynthifyShared/config"
	treev1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1/treev1connect"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/synthify/backend/worker/pkg/worker"
	"github.com/synthify/backend/worker/pkg/worker/llm"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadWorker()

	appCtx := app.Bootstrap(ctx, cfg.GCSUploadURLBase, cfg.FirebaseProjectID)
	store := appCtx.Store
	notifier := appCtx.Notifier

	var adkModel model.LLM
	var embedder *llm.GeminiClient
	llmCfg := config.LoadLLM()
	if llmCfg.Enabled() {
		var err error
		adkModel, err = gemini.NewModel(ctx, llmCfg.GeminiModel, &genai.ClientConfig{
			APIKey:  llmCfg.GeminiAPIKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Printf("Gemini model disabled: %v", err)
		}
		embedder = llm.NewGeminiClient(llmCfg)
	} else {
		log.Printf("Gemini API key not configured; worker will use deterministic fallback processing")
	}

	workerService, err := worker.NewWorkerWithNotifier(store, store, notifier, adkModel, embedder, embedder)
	if err != nil {
		log.Fatal(err)
	}
	planner := worker.NewPlanner(store, adkModel)
	evaluator := worker.NewJobEvaluator(store, embedder)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkerServiceHandler(worker.NewConnectHandler(workerService, store, planner, evaluator)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Synthify Worker listening on %s", addr)
	if err := http.ListenAndServe(addr, middleware.Logger(mux)); err != nil {
		log.Fatal(err)
	}
}
