package main

import (
	"errors"
	"fmt"
	"github.com/caarlos0/env/v10"
	"io"
	"io/ioutil"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type environment struct {
	Debug    bool     `env:"DEBUG" envDefault:"false"`
	Password string   `env:"STRATO_PASSWORD,required"`
	Domains  []string `env:"DOMAINS,required" envSeparator:","`
}

const (
	exitCodeConfigurationError int = 78
)

var (
	config        environment
	logger        *slog.Logger
	status        map[string]bool
	publicAddress string
)

func init() {
	err := env.Parse(&config)
	if err != nil {
		slog.Error(fmt.Sprintf("parsing env variables failed: %s", err.Error()))
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
	slog.Info("started strato dyndns updater")

	status = make(map[string]bool)
	for _, domain := range config.Domains {
		status[domain] = false
	}

	interval := time.Duration(5) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		slog.Debug("querying ifconfig.me")

		newAddress, err := getPublicIpAddress()
		if err != nil {
			slog.Error(fmt.Sprintf("querying ifconfig.me failed: %s", err))
		}
		slog.Debug(fmt.Sprintf("public ip address is: %s", newAddress))

		for _, domain := range config.Domains {
			trimmedDomain := strings.TrimSpace(domain)
			if status[domain] == false {
				err := updateDynDnsAddress(newAddress, trimmedDomain)
				if err != nil {
					slog.Error(fmt.Sprintf("updating strato dyndns failed: %s", err), "domain", trimmedDomain)
					continue
				}

				slog.Info("updated strato dyndns", "domain", trimmedDomain, "skip", publicAddress == newAddress && !status[domain])
				status[domain] = true

				continue
			}

			if publicAddress == newAddress {
				slog.Info("updating strato dyndns", "domain", trimmedDomain, "skip", publicAddress == newAddress && status[domain])
				continue
			}

			err := updateDynDnsAddress(newAddress, trimmedDomain)
			if err != nil {
				slog.Error(fmt.Sprintf("updating strato dyndns failed: %s", err), "domain", trimmedDomain)
				continue
			}

			slog.Info("updated strato dyndns", "domain", trimmedDomain, "skip", publicAddress == newAddress && status[domain])
			status[domain] = true
		}

		publicAddress = newAddress
		<-ticker.C
	}
}

func getPublicIpAddress() (string, error) {
	resp, err := http.Get("https://ifconfig.me/ip")
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ipAddress := strings.TrimSpace(string(body))
	if ipAddress == "" || net.ParseIP(ipAddress) == nil {
		return "", errors.New("no valid ip address found")
	}

	return ipAddress, nil
}

func updateDynDnsAddress(ipAddress string, domain string) error {
	url := fmt.Sprintf(
		"https://%s:%s@dyndns.strato.com/nic/update?hostname=%s&myip=%s",
		domain,
		config.Password,
		domain,
		ipAddress,
	)
	method := "GET"
	httpClient := &http.Client{}

	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(string(body), "good") && !strings.Contains(string(body), "nochg") {
		return errors.New(string(body))
	}

	return nil
}
