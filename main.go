package main

import (
	"bytes"
	"encoding/json"
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
	StagingDiscordWebhook    string `envconfig:"STAGING_DISCORD_WEBHOOK" required:"true"`
}

var Environment RequiredEnv

type webhook struct {
	discordWebhook string
}

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
		rd["content"] = md
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

	_, err = cli.Do(req)
	if err != nil {
		logrus.Info(err)
		hc.Response.Status = http.StatusInternalServerError
		return hh(hc)
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

	sw := &webhook{discordWebhook: Environment.StagingDiscordWebhook}
	mainAPI.Post("/staging", sw.handleWebhook)

	pw := &webhook{discordWebhook: Environment.ProductionDiscordWebhook}
	mainAPI.Post("/production", pw.handleWebhook)

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
