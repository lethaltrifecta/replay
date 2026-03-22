package commands

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/apiclient"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

const remoteAPITimeout = 30 * time.Second

var remoteStatusPollInterval = 2 * time.Second

func normalizeRemoteAPIBaseURL(server string) string {
	baseURL := strings.TrimRight(server, "/")
	if strings.HasSuffix(baseURL, "/api/v1") {
		return baseURL
	}
	return baseURL + "/api/v1"
}

func newRemoteAPIClient(server string) (*apiclient.ClientWithResponses, error) {
	client, err := apiclient.NewClientWithResponses(
		normalizeRemoteAPIBaseURL(server),
		apiclient.WithHTTPClient(&http.Client{Timeout: remoteAPITimeout}),
	)
	if err != nil {
		return nil, fmt.Errorf("build API client: %w", err)
	}
	return client, nil
}

func openapiClientUUID(id uuid.UUID) openapi_types.UUID {
	return openapi_types.UUID(id)
}

func remoteAPIErrorMessage(apiErr *apiclient.Error, body []byte) string {
	if apiErr != nil && apiErr.Error != "" {
		return apiErr.Error
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return "unexpected empty response body"
}

func remoteAPIStatusError(operation string, statusCode int, apiErr *apiclient.Error, body []byte) error {
	return fmt.Errorf("%s: server returned %d: %s", operation, statusCode, remoteAPIErrorMessage(apiErr, body))
}

func remoteGateCreateError(resp *apiclient.CreateGateCheckResp) error {
	switch {
	case resp == nil:
		return fmt.Errorf("submit gate check: empty response")
	case resp.JSON400 != nil:
		return remoteAPIStatusError("submit gate check", resp.StatusCode(), resp.JSON400, resp.Body)
	case resp.JSON404 != nil:
		return remoteAPIStatusError("submit gate check", resp.StatusCode(), resp.JSON404, resp.Body)
	case resp.JSON500 != nil:
		return remoteAPIStatusError("submit gate check", resp.StatusCode(), resp.JSON500, resp.Body)
	case resp.JSON503 != nil:
		return remoteAPIStatusError("submit gate check", resp.StatusCode(), resp.JSON503, resp.Body)
	default:
		return remoteAPIStatusError("submit gate check", resp.StatusCode(), nil, resp.Body)
	}
}

func remoteGateStatusError(resp *apiclient.GetGateStatusResp) error {
	switch {
	case resp == nil:
		return fmt.Errorf("poll status: empty response")
	case resp.JSON404 != nil:
		return remoteAPIStatusError("poll status", resp.StatusCode(), resp.JSON404, resp.Body)
	case resp.JSON500 != nil:
		return remoteAPIStatusError("poll status", resp.StatusCode(), resp.JSON500, resp.Body)
	default:
		return remoteAPIStatusError("poll status", resp.StatusCode(), nil, resp.Body)
	}
}

func remoteGateReportError(resp *apiclient.GetGateReportResp) error {
	switch {
	case resp == nil:
		return fmt.Errorf("fetch report: empty response")
	case resp.JSON404 != nil:
		return remoteAPIStatusError("fetch report", resp.StatusCode(), resp.JSON404, resp.Body)
	case resp.JSON500 != nil:
		return remoteAPIStatusError("fetch report", resp.StatusCode(), resp.JSON500, resp.Body)
	default:
		return remoteAPIStatusError("fetch report", resp.StatusCode(), nil, resp.Body)
	}
}

func runGateCheckRemote(cmd *cobra.Command, server, baselineTraceID, model, provider string, requestHeaders map[string]string, threshold float64) error {
	ctx := commandContext(cmd)
	client, err := newRemoteAPIClient(server)
	if err != nil {
		return err
	}

	cmd.Printf("Submitting gate check to %s...\n", server)

	request := apiclient.CreateGateCheckJSONRequestBody{
		BaselineTraceId: baselineTraceID,
		Model:           model,
		Threshold:       float32Ptr(float32(threshold)),
	}
	if provider != "" {
		request.Provider = &provider
	}
	if len(requestHeaders) > 0 {
		headers := requestHeaders
		request.RequestHeaders = &headers
	}

	createResp, err := client.CreateGateCheckWithResponse(ctx, request)
	if err != nil {
		return fmt.Errorf("submit gate check: %w", err)
	}
	if createResp.JSON202 == nil || createResp.JSON202.ExperimentId == nil {
		return remoteGateCreateError(createResp)
	}

	experimentID := *createResp.JSON202.ExperimentId
	cmd.Printf("Experiment %s submitted, polling for results...\n", experimentID.String())

	ticker := time.NewTicker(remoteStatusPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		statusResp, err := client.GetGateStatusWithResponse(ctx, experimentID)
		if err != nil {
			return fmt.Errorf("poll status: %w", err)
		}
		if statusResp.JSON200 == nil || statusResp.JSON200.Status == nil {
			return remoteGateStatusError(statusResp)
		}

		status := *statusResp.JSON200.Status
		switch status {
		case storage.StatusRunning:
			progress := float32(0)
			if statusResp.JSON200.Progress != nil {
				progress = *statusResp.JSON200.Progress
			}
			cmd.Printf("  Progress: %.0f%%\n", float64(progress)*100)
			continue
		case storage.StatusCompleted, storage.StatusFailed:
			return fetchAndPrintRemoteReport(ctx, client, cmd, experimentID, model)
		default:
			cmd.Printf("  Status: %s\n", status)
		}
	}
}

func fetchAndPrintRemoteReport(ctx context.Context, client *apiclient.ClientWithResponses, cmd *cobra.Command, experimentID openapi_types.UUID, model string) error {
	reportResp, err := client.GetGateReportWithResponse(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("fetch report: %w", err)
	}
	if reportResp.JSON200 == nil {
		return remoteGateReportError(reportResp)
	}

	report := reportResp.JSON200
	displayModel := remoteVariantModel(report, model)
	status := ""
	if report.Status != nil {
		status = *report.Status
	}
	baselineTraceID := ""
	if report.BaselineTraceId != nil {
		baselineTraceID = *report.BaselineTraceId
	}

	cmd.Printf("\nGate Check Result\n")
	cmd.Printf("=================\n")
	cmd.Printf("Baseline:   %s\n", baselineTraceID)
	cmd.Printf("Variant:    %s\n", displayModel)
	cmd.Printf("Status:     %s\n", status)

	if report.SimilarityScore != nil {
		cmd.Printf("Similarity: %.4f\n", *report.SimilarityScore)
	}
	if report.Verdict != nil && *report.Verdict != "" {
		cmd.Printf("Verdict:    %s\n", verdictDisplay(*report.Verdict))
	}
	if report.TokenDelta != nil {
		cmd.Printf("Token Delta: %+d\n", *report.TokenDelta)
	}
	if report.LatencyDelta != nil {
		cmd.Printf("Latency Delta: %+dms\n", *report.LatencyDelta)
	}
	cmd.Printf("Experiment: %s\n", experimentID.String())

	if report.Verdict != nil && *report.Verdict == "fail" {
		return ErrGateFailed
	}
	if status == storage.StatusFailed {
		if report.Error != nil && *report.Error != "" {
			return fmt.Errorf("experiment failed: %s", *report.Error)
		}
		return fmt.Errorf("experiment failed without producing a verdict")
	}

	return nil
}

func remoteVariantModel(report *apiclient.ExperimentReport, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if report == nil || report.Runs == nil {
		return ""
	}
	for _, run := range *report.Runs {
		if run.RunType == nil || *run.RunType != apiclient.ExperimentRunRunTypeVariant {
			continue
		}
		if run.VariantConfig != nil && run.VariantConfig.Model != nil && *run.VariantConfig.Model != "" {
			return *run.VariantConfig.Model
		}
	}
	return ""
}

func float32Ptr(value float32) *float32 {
	return &value
}
