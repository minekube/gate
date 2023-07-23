package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/rs/xid"
	"go.minekube.com/gate/pkg/internal/randstr"
	"go.minekube.com/gate/pkg/version"
)

func randomEndpointName(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	name, err := fetchEndpointName(ctx)
	if err != nil {
		logr.FromContextOrDiscard(ctx).V(1).Error(err, "failed to fetch random endpoint name")
		return xid.New().String()
	}
	return name
}

func fetchEndpointName(ctx context.Context) (string, error) {
	const url = "https://randomname.minekube.net"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("could not create request to %s: %w", url, err)
	}
	req.Header = version.UserAgentHeader()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not generate endpoint name: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %w", err)
	}
	// Add random suffix
	return fmt.Sprintf("%s-%s", string(body), randstr.String(3)), nil
}
