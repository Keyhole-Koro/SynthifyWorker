package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/synthify/backend/apps/worker/pkg/worker"
	"github.com/synthify/backend/apps/worker/pkg/worker/llm"
	"github.com/synthify/backend/packages/shared/app"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/config"
	treev1connect "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
	"github.com/synthify/backend/packages/shared/job/log"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/postgres"
	"github.com/synthify/backend/packages/shared/storage"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadWorker()
	fs := storage.NewFileSystem(cfg.GCSFuseMountPath)
	appLogger := applog.NewStdLogger()

	appCtx := app.Bootstrap(ctx, cfg.GCSUploadURLBase, cfg.FirebaseProjectID, appLogger)
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
			appLogger.Error(ctx, "worker.adk_model_init_failed", err, map[string]any{"model": llmCfg.GeminiModel})
		}
		embedder = llm.NewGeminiClient(llmCfg, fs)
	} else {
		appLogger.Info(ctx, "worker.gemini_disabled", map[string]any{"reason": "no api key"})
	}

	workerService, err := worker.NewWorkerWithNotifier(store, store, notifier, adkModel, embedder, embedder, fs, appLogger)
	if err != nil {
		log.Fatal(err)
	}
	planner := worker.NewPlanner(store, adkModel, appLogger)
	evaluator := worker.NewJobEvaluator(store, embedder, appLogger)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkerServiceHandler(worker.NewConnectHandler(workerService, store, planner, evaluator, appLogger)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	appLogger.Info(ctx, "worker.started", map[string]any{"addr": addr})
	h := middleware.Recover(appLogger, middleware.Logger(appLogger, withJobLogger(jobLogger, mux)))
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
