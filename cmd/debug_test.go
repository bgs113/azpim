package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

// replyWith is a transport that answers every request with a canned response.
type replyWith struct {
	status int
	header http.Header
	body   string
}

func (r replyWith) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Header:     r.header,
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

// send makes one GET through an azcore pipeline that uses transport.
func send(t *testing.T, transport policy.Transporter) *http.Response {
	t.Helper()
	pl := runtime.NewPipeline("azpim-test", "v0", runtime.PipelineOptions{}, &policy.ClientOptions{
		Transport: transport,
		Retry:     policy.RetryOptions{MaxRetries: -1},
	})
	req, err := runtime.NewRequest(context.Background(), http.MethodGet,
		"https://management.azure.com/providers/Microsoft.Authorization/roleEligibilityScheduleInstances?api-version=2020-10-01&$filter=asTarget()")
	if err != nil {
		t.Fatal(err)
	}
	req.Raw().Header.Set("Authorization", "Bearer secret-token")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestEnableDebugLog(t *testing.T) {
	var buf bytes.Buffer
	enableDebugLog(&buf)
	t.Cleanup(func() { azlog.SetListener(nil) })

	resp := send(t, replyWith{status: http.StatusOK, header: http.Header{"X-Ms-Request-Id": {"req-123"}}, body: "{}"})
	_ = resp.Body.Close()

	got := buf.String()
	for _, want := range []string{"[debug", "Request: ==> OUTGOING REQUEST", "Response: ==> REQUEST/RESPONSE", "roleEligibilityScheduleInstances", "req-123"} {
		if !strings.Contains(got, want) {
			t.Errorf("debug log is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret-token") {
		t.Errorf("debug log leaks the bearer token:\n%s", got)
	}
}

func TestFriendlyErrorCorrelationID(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{"correlation ID", http.Header{"X-Ms-Correlation-Request-Id": {"corr-1"}, "X-Ms-Request-Id": {"req-1"}}, " (correlation ID corr-1)"},
		{"request ID only", http.Header{"X-Ms-Request-Id": {"req-1"}}, " (correlation ID req-1)"},
		{"neither", http.Header{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := send(t, replyWith{status: http.StatusForbidden, header: tt.header,
				body: `{"error":{"code":"AuthorizationFailed","message":"no"}}`})
			defer func() { _ = resp.Body.Close() }()
			// Callers wrap Azure's error, as pim.Activate does.
			err := fmt.Errorf("activate role: %w", runtime.NewResponseError(resp))
			want := "permission denied" + tt.want
			if got := friendlyError(err); !strings.HasSuffix(got, want) || strings.Contains(got, "\n") {
				t.Errorf("friendlyError() = %q, want one line ending in %q", got, want)
			}
		})
	}

	if got := correlationID(&azcore.ResponseError{StatusCode: 403}); got != "" {
		t.Errorf("correlationID without a response = %q, want empty", got)
	}
}
