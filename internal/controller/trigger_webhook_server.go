package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// TriggerWebhookServer handles inbound HTTP requests that fire webhook-type ArkEvents.
//
// Endpoint: POST /triggers/{namespace}/{name}/fire
//
// Authentication: pass the trigger's token as a Bearer token in the Authorization
// header or as the `token` query parameter. The token is stored in a Secret named
// <trigger-name>-webhook-token in the same namespace.
//
// The request body is optional JSON. Fields are available in target input templates
// as {{ .trigger.body.<field> }}.
type TriggerWebhookServer struct {
	reconciler *ArkEventReconciler
}

// NewTriggerWebhookServer returns a new TriggerWebhookServer.
func NewTriggerWebhookServer(r *ArkEventReconciler) *TriggerWebhookServer {
	return &TriggerWebhookServer{reconciler: r}
}

// ServeHTTP implements http.Handler.
func (s *TriggerWebhookServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// Expect: /triggers/{namespace}/{name}/fire
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "triggers" || parts[3] != "fire" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	namespace, name := parts[1], parts[2]

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Load the trigger.
	trigger := &arkonisv1alpha1.ArkEvent{}
	if err := s.reconciler.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, trigger); err != nil {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	if trigger.Spec.Source.Type != arkonisv1alpha1.TriggerSourceWebhook {
		http.Error(w, "trigger is not of type webhook", http.StatusBadRequest)
		return
	}

	if trigger.Spec.Suspended {
		http.Error(w, "trigger is suspended", http.StatusServiceUnavailable)
		return
	}

	// Validate token.
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if !s.validToken(ctx, trigger, token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse optional JSON body.
	var body map[string]any
	if r.ContentLength != 0 {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
	}

	now := time.Now().UTC()
	fireCtx := FireContext{
		Name:    trigger.Name,
		FiredAt: now.Format(time.RFC3339),
		Body:    body,
	}

	if err := s.reconciler.fire(ctx, trigger, fireCtx); err != nil {
		logger.Error(err, "firing webhook trigger", "trigger", name)
		http.Error(w, fmt.Sprintf("fire failed: %v", err), http.StatusInternalServerError)
		return
	}

	nowMeta := metav1.NewTime(now)
	trigger.Status.LastFiredAt = &nowMeta
	trigger.Status.FiredCount++
	trigger.Status.ObservedGeneration = trigger.Generation
	if err := s.reconciler.Status().Update(ctx, trigger); err != nil {
		logger.Error(err, "updating trigger status after webhook fire")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"fired":   true,
		"firedAt": now.Format(time.RFC3339),
		"trigger": name,
		"targets": len(trigger.Spec.Targets),
	})
}

// validToken checks the provided token against the one stored in the trigger's webhook token Secret.
func (s *TriggerWebhookServer) validToken(ctx context.Context, trigger *arkonisv1alpha1.ArkEvent, token string) bool {
	if token == "" {
		return false
	}
	stored, err := webhookTokenFromSecret(ctx, s.reconciler.Client, trigger)
	if err != nil {
		return false
	}
	return token == stored
}

// webhookTokenFromSecret reads the token from the trigger's token Secret.
// Secret name: <trigger-name>-webhook-token, key: token.
func webhookTokenFromSecret(ctx context.Context, c client.Client, trigger *arkonisv1alpha1.ArkEvent) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      trigger.Name + "-webhook-token",
		Namespace: trigger.Namespace,
	}, secret); err != nil {
		return "", err
	}
	return string(secret.Data["token"]), nil
}

// bearerToken extracts the Bearer token from the Authorization header.
func bearerToken(r *http.Request) string {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return token
	}
	return ""
}
