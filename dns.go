package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type StratoDynDnsClient struct {
	*http.Client
}

func NewStratoDynDnsClient() *StratoDynDnsClient {
	return &StratoDynDnsClient{&http.Client{}}
}

func (c StratoDynDnsClient) UpdateRecords(ctx context.Context, domain, ipAddress, masterPassword string) error {
	url := fmt.Sprintf(
		"https://%s:%s@dyndns.strato.com/nic/update?hostname=%s&myip=%s",
		domain,
		masterPassword,
		domain,
		ipAddress,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(string(body), "good") && !strings.Contains(string(body), "nochg") {
		return errors.New(string(body))
	}

	return nil
}
