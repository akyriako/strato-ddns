package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ipq "github.com/akyriako/ipquery-go"
	"github.com/caarlos0/env/v10"
)

type Config struct {
	Debug           bool          `env:"DEBUG" envDefault:"false"`
	Password        string        `env:"STRATO_PASSWORD,required"`
	Domains         []string      `env:"DOMAINS,required" envSeparator:","`
	IPQueryUser     string        `env:"IP_QUERY_USER,required"`
	IPQueryPassword string        `env:"IP_QUERY_PASSWORD,required"`
	IPQueryURL      string        `env:"IP_QUERY_URL,required"`
	Timeout         time.Duration `env:"TIMEOUT" envDefault:"500"`
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
	ipqc        *ipq.Client
)

func init() {
	err := env.Parse(&config)
	if err != nil {
		slog.Error(fmt.Sprintf("parsing env variables failed: %s", err.Error()))
		os.Exit(exitCodeConfigurationError)
	}

	ipqc, err = ipq.NewClient(
		config.IPQueryURL,
		ipq.WithBasicAuth(config.IPQueryUser, config.IPQueryPassword),
		ipq.WithTimeout(config.Timeout),
	)
	if err != nil {
		slog.Error(fmt.Sprintf("initializing ipquery client failed: %s", err.Error()))
		os.Exit(exitCodeConfigurationError)
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
	ip, provider, err := ipqc.GetOwnIP()
	if err != nil {
		slog.Error("retrieving own ip address failed: " + err.Error())
		return
	}

	if lastKnownIp == ip {
		slog.Info("no change in ip address", "ip", ip, "provider", provider)
		return
	}

	slog.Info("retrieved new ip address", "ip", ip, "provider", provider)

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
