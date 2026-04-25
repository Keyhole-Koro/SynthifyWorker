package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/app"
	"github.com/Keyhole-Koro/SynthifyShared/config"
	treev1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1/treev1connect"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/synthify/backend/worker/pkg/worker"
	"google.golang.org/adk/model"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadWorker()

	store := app.InitStore(ctx, app.PublicUploadURLGenerator(cfg.GCSUploadURLBase))
	notifier := jobstatus.NewNotifier(ctx, cfg.FirebaseProjectID)
	
	// Initialize ADK model (wrapper for Gemini)
	// In a real scenario, this would use model.NewGoogleModel
	var adkModel model.LLM 

	workerService, err := worker.NewWorkerWithNotifier(store, store, notifier, adkModel)
	if err != nil {
		log.Fatal(err)
	}
	planner := worker.NewPlanner(store, adkModel)
	evaluator := worker.NewJobEvaluator(store, adkModel)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkerServiceHandler(worker.NewConnectHandler(workerService, store, planner, evaluator, cfg.InternalWorkerToken)))
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
