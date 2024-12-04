package main

import (
	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"context"
	"github.com/caarlos0/env/v11"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

type config struct {
	Token                   string `env:"TOKEN,required"`
	QueuePath               string `env:"QUEUE_PATH,required"`
	HttpTarget              string `env:"HTTP_TARGET,required"`
	HttpTargetAuthorization string `env:"HTTP_TARGET_AUTHORIZATION,required"`
	Port                    string `env:"PORT" envDefault:"8080"`
}

func main() {
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	var cfg config
	err := env.Parse(&cfg)
	if err != nil {
		slog.Error("Failed to read configuration from environment", "reason", err)
		return 1
	}

	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		slog.Error("Failed to create client", "reason", err)
		return 1
	}
	defer client.Close()

	http.HandleFunc("POST /waqu", handleRequest(client, cfg))

	address := ":" + cfg.Port
	slog.Info("Starting server", "address", address)

	err = http.ListenAndServe(address, nil)
	if err != nil {
		slog.Error("Failed to start server", "reason", err)
		return 1
	}

	return 0
}

func handleRequest(client *cloudtasks.Client, cfg config) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != cfg.Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("Failed to read request body", "reason", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		req := &taskspb.CreateTaskRequest{
			Parent: cfg.QueuePath,
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{
					HttpRequest: &taskspb.HttpRequest{
						HttpMethod: taskspb.HttpMethod_POST,
						Url:        cfg.HttpTarget,
						Headers: map[string]string{
							"Authorization": cfg.HttpTargetAuthorization,
							"Content-Type":  "application/json",
						},
						Body: body,
					},
				},
			},
		}

		resp, err := client.CreateTask(context.Background(), req)
		if err != nil {
			slog.Error("Failed to create task", "reason", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		slog.Info("Enqueued message", "task", resp.Name[strings.LastIndex(resp.Name, "/")+1:])
	}
}
