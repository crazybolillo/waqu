package main

import (
	"bytes"
	"cloud.google.com/go/pubsub"
	"context"
	"encoding/json"
	"github.com/caarlos0/env/v11"
	"github.com/zoftko/gowhat/webhook"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

type config struct {
	ProjectID    string `env:"PROJECT_ID,required"`
	TopicId      string `env:"TOPIC_ID,required"`
	Token        string `env:"TOKEN,required"`
	Port         string `env:"PORT" envDefault:"8080"`
	IgnoreStatus bool   `env:"IGNORE_STATUS" envDefault:"true"`
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

	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		slog.Error("Failed to create client", "reason", err)
		return 1
	}
	defer client.Close()

	topic := client.Topic(cfg.TopicId)
	ok, err := topic.Exists(context.Background())
	if err != nil {
		slog.Error("Failed to check if topic exists", "reason", err)
		return 1
	}
	if !ok {
		slog.Error("Topic does not exist", "id", cfg.TopicId)
		return 1
	}

	http.HandleFunc("POST /waqu", handleRequest(topic, cfg))

	address := ":" + cfg.Port
	slog.Info("Starting server", "address", address)

	err = http.ListenAndServe(address, nil)
	if err != nil {
		slog.Error("Failed to start server", "reason", err)
		return 1
	}

	return 0
}

func isMessageEvent(content io.Reader) (bool, error) {
	var notification webhook.Notification
	decoder := json.NewDecoder(content)
	err := decoder.Decode(&notification)
	if err != nil {
		return false, nil
	}

	for _, entry := range notification.Entry {
		for _, change := range entry.Changes {
			if len(change.Value.Messages) > 0 {
				return true, nil
			}
		}
	}

	return false, nil
}

func handleRequest(topic *pubsub.Topic, cfg config) func(http.ResponseWriter, *http.Request) {
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

		if cfg.IgnoreStatus {
			isMsg, err := isMessageEvent(bytes.NewReader(body))
			if err != nil {
				slog.Error("Failed to decode request body", "reason", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if !isMsg {
				return
			}
		}

		res := topic.Publish(r.Context(), &pubsub.Message{
			Data: body,
		})
		id, err := res.Get(r.Context())
		if err != nil {
			slog.Error("Failed to publish message", "reason", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		slog.Info("Enqueued message", "id", id)
	}
}
