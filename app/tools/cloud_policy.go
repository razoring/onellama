//go:build windows || darwin

package tools

import (
	"context"
	"errors"

	"github.com/ollama/ollama/api"
	internalcloud "github.com/ollama/ollama/internal/cloud"
)

// ensureCloudEnabledForTool checks cloud policy from the connected Ollama server.
// If policy cannot be determined, this fails closed and blocks the operation.
func ensureCloudEnabledForTool(ctx context.Context, operation string) error {
	// Reuse shared message formatting; policy evaluation is still done via
	// the connected server's /api/status endpoint below.
	disabledMessage := internalcloud.DisabledError(operation)

	client, err := api.ClientFromEnvironment()
	if err != nil {
		return errors.New(disabledMessage + " (unable to verify server cloud policy)")
	}

	status, err := client.CloudStatusExperimental(ctx)
	if err != nil {
		return errors.New(disabledMessage + " (unable to verify server cloud policy)")
	}

	if status.Cloud.Disabled {
		return errors.New(disabledMessage)
	}

	return nil
}

// IsCloudDisabled returns true if Ollama Cloud is disabled on the connected server,
// or if server status cannot be queried.
func IsCloudDisabled(ctx context.Context) bool {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return true
	}
	status, err := client.CloudStatusExperimental(ctx)
	if err != nil {
		return true
	}
	return status.Cloud.Disabled
}
