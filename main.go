package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"github.com/nitrictech/go-sdk/faas"
	"github.com/nitrictech/go-sdk/resources"
)

type RequiredEnv struct {
	ProductionDiscordWebhook string `envconfig:"PRODUCTION_DISCORD_WEBHOOK" required:"true"`
	ProductionUsername       string `envconfig:"PRODUCTION_USERNAME" required:"true"`
	ProductionPassword       string `envconfig:"PRODUCTION_PASSWORD" required:"true"`
	StagingDiscordWebhook    string `envconfig:"STAGING_DISCORD_WEBHOOK" required:"true"`
	StagingUsername          string `envconfig:"STAGING_USERNAME" required:"true"`
	StagingPassword          string `envconfig:"STAGING_PASSWORD" required:"true"`
}

var Environment RequiredEnv

type webhook struct {
	discordWebhook string
	projectName    string
	username       string
	password       string
}

/*
var foo = map[string]any{"incident": map[string]any{
	"apigee_url": "http://www.example.com",
	"condition": map[string]any{
		"conditionThreshold": map[string]any{
			"comparison":     "COMPARISON_GT",
			"duration":       "0s",
			"filter":         "metric.type=\"test.googleapis.com/metric\" resource.type=\"example_resource\"",
			"thresholdValue": 0.5,
			"trigger":        map[string]any{"count": 1}},
		"displayName": "Example condition",
		"name":        "projects/12345/alertPolicies/12345/conditions/12345",
	},
	"condition_name": "Example condition",
	"documentation":  "Test documentation",
	"ended_at":       0, "incident_id": "12345",
	"metadata": map[string]any{
		"system_labels": map[string]any{"example": "label"},
		"user_labels":   map[string]any{"example": "label"}},
	"metric": map[string]any{
		"displayName": "Test Metric",
		"labels":      map[string]any{"example": "label"},
		"type":        "test.googleapis.com/metric"},
	"observed_value":     "1.0",
	"policy_name":        "projects/12345/alertPolicies/12345",
	"policy_user_labels": map[string]any{"example": "label"},
	"resource": map[string]any{
		"labels": map[string]any{"example": "label"},
		"type":   "example_resource"},
	"resource_display_name":      "Example Resource",
	"resource_id":                "12345",
	"resource_name":              "projects/12345/example_resources/12345",
	"resource_type_display_name": "Example Resource Type",
	"scoping_project_id":         "12345",
	"scoping_project_number":     12345,
	"started_at":                 0,
	"state":                      "OPEN",
	"summary":                    "Test Incident",
	"threshold_value":            "0.5",
	"url":                        "http://www.example.com"},
	"version": "test"}
*/

func (w *webhook) handleWebhook(hc *faas.HttpContext, hh faas.HttpHandler) (*faas.HttpContext, error) {
	md := map[string]any{}
	rd := map[string]any{
		"content": "",
	}

	err := json.Unmarshal(hc.Request.Data(), &md)
	if err != nil {
		// assume not json
		rd["content"] = string(hc.Request.Data())
	} else {
		incident := md["incident"].(map[string]any)
		fmt.Println("incident ", incident)

		msgFmt := "New incident in %s\nView Error details %s\nAll Error reports https://console.cloud.google.com/errors?project=%s"

		rd["content"] = fmt.Sprintf(msgFmt,
			incident["resource_name"],
			incident["url"],
			w.projectName)
	}

	b, err := json.Marshal(rd)
	if err != nil {
		logrus.Info(err)
		hc.Response.Status = http.StatusBadRequest
		return hh(hc)
	}

	req, err := http.NewRequest(http.MethodPost, w.discordWebhook, bytes.NewReader(b))
	if err != nil {
		logrus.Info(err)
		hc.Response.Status = http.StatusBadRequest
		return hh(hc)
	}
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := cli.Do(req)
	if err != nil {
		logrus.Info(err)
		hc.Response.Status = http.StatusInternalServerError
		return hh(hc)
	}

	fmt.Println("response ", resp.Status)

	return hh(hc)
}

func (w *webhook) basicAuth(hc *faas.HttpContext, hh faas.HttpHandler) (*faas.HttpContext, error) {
	r := http.Request{Header: hc.Request.Headers()}

	authV, ok := r.Header["X-Forwarded-Authorization"]
	if ok && strings.HasPrefix(authV[0], "Basic ") {
		r.Header["Authorization"] = authV
	}

	u, p, ok := r.BasicAuth()
	if !ok {
		logrus.Info("auth not provided")
		hc.Response.Status = http.StatusForbidden

		return hc, errors.New("auth not provided")
	}

	if u != w.username || p != w.password {
		logrus.Info("wrong auth")
		hc.Response.Status = http.StatusForbidden

		return hc, errors.New("wrong auth")
	}

	return hh(hc)
}

func run() error {
	err := envconfig.Process("", &Environment)
	if err != nil {
		return err
	}

	mainAPI, err := resources.NewApi("webhook")
	if err != nil {
		return err
	}

	sw := &webhook{
		discordWebhook: Environment.StagingDiscordWebhook,
		projectName:    "nitric-deploy-staging",
		username:       Environment.StagingUsername,
		password:       Environment.StagingPassword,
	}
	mainAPI.Post("/staging", faas.ComposeHttpMiddlware(sw.basicAuth, sw.handleWebhook))

	pw := &webhook{
		discordWebhook: Environment.ProductionDiscordWebhook,
		projectName:    "nitric-deploy-production",
		username:       Environment.ProductionUsername,
		password:       Environment.ProductionPassword,
	}
	mainAPI.Post("/production", faas.ComposeHttpMiddlware(pw.basicAuth, pw.handleWebhook))

	return resources.Run()
}

func main() {
	if err := run(); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			logrus.Info("Shutting down")
		} else {
			panic(err)
		}
	}
}
