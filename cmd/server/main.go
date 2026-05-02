package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/synthify/backend/packages/shared/app"
	"github.com/synthify/backend/packages/shared/config"
	treev1connect "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
	"github.com/synthify/backend/packages/shared/joblog"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/postgres"
	"github.com/synthify/backend/apps/worker/pkg/worker"
	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
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

	jobLogger := postgres.NewDBLogger(store)

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
	h := middleware.Recover(middleware.Logger(withJobLogger(jobLogger, mux)))
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func withJobLogger(l joblog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := joblog.WithLogger(r.Context(), l)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
