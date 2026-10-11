package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/domain"

	"github.com/stretchr/testify/require"
)

// C5: the TailLogs/StopTailLogs/QueryLogs validation-failure paths must emit
// the response envelope with a message, not a literal `{}`.

// stubApplicationService embeds the interface so the validation-failure
// paths (which return before touching the service) need no method bodies.
type stubApplicationService struct {
	domain.ApplicationService
}

type stubContainerLogService struct {
	domain.ContainerLogService
}

func newApplicationValidationHandler() *ApplicationHandler {
	return NewApplicationHandler(
		stubApplicationService{},
		stubContainerLogService{},
		request.NewJSONDecoder(),
		response.NewJSONWriter(stubLogger{}),
		validator.NewValidator(),
	)
}

func assertValidationEnvelope(t *testing.T, rec *httptest.ResponseRecorder) response.Response {
	t.Helper()
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	var body response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"body must decode as the response envelope, got %q", rec.Body.String())
	require.NotEmpty(t, body.Message, "envelope message must not be empty (was `{}` before the fix)")
	require.NotNil(t, body.Errors, "envelope must carry the field errors map")
	return body
}

func validationRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/applications/7/logs/tail", strings.NewReader(body))
	req.SetPathValue("id", "7")
	return req
}

func TestApplicationHandler_TailLogs_ValidationError(t *testing.T) {
	h := newApplicationValidationHandler()
	rec := httptest.NewRecorder()

	// tail < 1 violates `validate:"omitempty,min=1,max=5000"`.
	h.TailLogs(rec, validationRequest(t, `{"tail":-1}`))

	assertValidationEnvelope(t, rec)
}

func TestApplicationHandler_StopTailLogs_ValidationError(t *testing.T) {
	h := newApplicationValidationHandler()
	rec := httptest.NewRecorder()

	// stream_id is `validate:"required"`.
	h.StopTailLogs(rec, validationRequest(t, `{}`))

	assertValidationEnvelope(t, rec)
}

func TestApplicationHandler_QueryLogs_ValidationError(t *testing.T) {
	h := newApplicationValidationHandler()
	rec := httptest.NewRecorder()

	// tail < 1 violates `validate:"omitempty,min=1,max=5000"`.
	h.QueryLogs(rec, validationRequest(t, `{"tail":-1}`))

	assertValidationEnvelope(t, rec)
}
