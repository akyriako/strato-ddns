package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	Debug           bool     `env:"DEBUG" envDefault:"false"`
	Password        string   `env:"STRATO_PASSWORD,required"`
	Domains         []string `env:"DOMAINS,required" envSeparator:","`
	IPQueryUser     string   `env:"IP_QUERY_USER,required"`
	IPQueryPassword string   `env:"IP_QUERY_PASSWORD,required"`
	IPQueryURL      string   `env:"IP_QUERY_URL,required"`
}

const (
	exitCodeConfigurationError int = 78
)

var (
	config      Config
	logger      *slog.Logger
	status      map[string]string
	sc          *StratoDynDnsClient
	lastKnownIp string
	httpClient  *http.Client
)

func init() {
	err := env.Parse(&config)
	if err != nil {
		slog.Error(fmt.Sprintf("parsing env variables failed: %s", err.Error()))
		os.Exit(exitCodeConfigurationError)
	}

	httpClient = &http.Client{
		Timeout: time.Millisecond * 500,
	}

	levelInfo := slog.LevelInfo
	if config.Debug {
		levelInfo = slog.LevelDebug
	}

	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelInfo,
	}))

	slog.SetDefault(logger)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("starting strato dyndns updater")

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		<-sigChan

		slog.Warn("termination signal received, shutting down gracefully...")
		cancel()
	}()

	status = make(map[string]string)
	for _, domain := range config.Domains {
		status[domain] = ""
	}

	interval := time.Duration(5) * time.Minute
	first := time.After(0)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sc = NewStratoDynDnsClient()

	for {
		select {
		case <-first:
			updateRecordSets(ctx)
		case <-ticker.C:
			updateRecordSets(ctx)
		case <-ctx.Done():
			slog.Info(fmt.Sprintf("stopped strato dyndns updater"))
			return
		}
	}
}

func updateRecordSets(ctx context.Context) {
	ip, err := getOwnIP()
	if err != nil {
		slog.Error("retrieving own ip address failed: " + err.Error())
		return
	}

	if lastKnownIp == ip {
		slog.Info("no changes in ip address", "ip", ip)
		return
	}

	slog.Info("retrieved new ip address", "ip", ip)

	for _, domain := range config.Domains {
		trimmedDomain := strings.TrimSpace(domain)
		if status[domain] == ip {
			slog.Info("updating dyndns records skipped", "domain", trimmedDomain)
			continue
		}

		slog.Info("updating dyndns records", "domain", trimmedDomain)

		err := sc.UpdateRecords(ctx, trimmedDomain, ip, config.Password)
		if err != nil {
			slog.Error(fmt.Sprintf("updating dyndns records failed: %s", err.Error()), "domain", trimmedDomain)
		}

		status[domain] = ip
	}

	lastKnownIp = ip
}

func getOwnIP() (string, error) {
	req, err := http.NewRequest(http.MethodGet, config.IPQueryURL, nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(
		config.IPQueryUser,
		config.IPQueryPassword,
	)
	req.Header.Set("Content-Type", "application/text")

	httpResponse, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != 200 {
		return "", fmt.Errorf("http status %d", httpResponse.StatusCode)
	}

	httpBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return "", err
	}

	ipStr := string(httpBody)
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return "", fmt.Errorf("failed to parse ip address")
	}
	// Normalize IPv4-in-IPv6 form
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), nil
	}

	return ipStr, nil
}
