package test

import (
	"context"
	"testing"
	"time"

	"github.com/vertesan/solis-server/internal/proto/papi"
)

func TestHealthPingScenario(t *testing.T) {
	// A liveness probe must work before login and without game-version headers.
	client := papi.NewHealthClient(testConfig.GrpcConn)
	for _, service := range []string{"", "api.Health", "api.Showcase"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		response, err := client.Check(ctx, &papi.HealthCheckRequest{Service: service})
		cancel()
		if err != nil || response == nil {
			t.Fatalf("Health.Check(%q) failed: %v", service, err)
		}
	}
}
