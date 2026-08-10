package cognitiontransport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type server struct {
	environment  cognition.Environment
	completion   cognitionruntime.CompletionEvaluator
	authenticate Authenticator
}

func NewHandler(
	environment cognition.Environment,
	completion cognitionruntime.CompletionEvaluator,
	authenticate Authenticator,
) (http.Handler, error) {
	if nilInterface(environment) || nilInterface(completion) {
		return nil, fmt.Errorf("cognition environment or completion evaluator is not configured")
	}
	if authenticate == nil {
		return nil, fmt.Errorf("cognition environment authenticator is not configured")
	}
	return &server{environment: environment, completion: completion, authenticate: authenticate}, nil
}

func (server *server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeWireError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is registered.")
		return
	}
	if err := server.authenticate(request); err != nil {
		writeWireError(writer, http.StatusUnauthorized, "unauthorized", "Environment credential was rejected.")
		return
	}
	switch request.URL.Path {
	case startPath:
		server.start(writer, request)
	case applyPath:
		server.apply(writer, request)
	case evaluatePath:
		server.evaluate(writer, request)
	default:
		writeWireError(writer, http.StatusNotFound, "not_found", "Environment operation is not registered.")
	}
}

func (server *server) start(writer http.ResponseWriter, request *http.Request) {
	var input startRequest
	if err := decodeRequest(request, &input); err != nil || input.Protocol != ProtocolVersionV1 {
		writeWireError(writer, http.StatusBadRequest, "invalid_request", "Start request is invalid.")
		return
	}
	if err := input.Scenario.Validate(); err != nil {
		writeWireError(writer, http.StatusBadRequest, "invalid_scenario", "Scenario reference is invalid.")
		return
	}
	transition, err := server.environment.Start(request.Context(), input.Scenario)
	if err != nil {
		writeWireError(writer, http.StatusConflict, "start_rejected", "Environment rejected episode start.")
		return
	}
	if err := transition.ValidateStart(); err != nil {
		writeWireError(writer, http.StatusInternalServerError, "invalid_transition", "Environment returned an invalid start transition.")
		return
	}
	writeWire(writer, http.StatusOK, wireResponse{Protocol: ProtocolVersionV1, Transition: &transition})
}

func (server *server) apply(writer http.ResponseWriter, request *http.Request) {
	var input applyRequest
	if err := decodeRequest(request, &input); err != nil || input.Protocol != ProtocolVersionV1 {
		writeWireError(writer, http.StatusBadRequest, "invalid_request", "Apply request is invalid.")
		return
	}
	if err := input.Episode.Validate(); err != nil {
		writeWireError(writer, http.StatusBadRequest, "invalid_episode", "Episode reference is invalid.")
		return
	}
	if err := input.Expected.Validate(); err != nil {
		writeWireError(writer, http.StatusBadRequest, "invalid_revision", "Expected revision is invalid.")
		return
	}
	transition, err := server.environment.Apply(
		request.Context(), input.Episode, input.Expected, input.Action,
	)
	if err != nil {
		var failure cognition.ActionFailure
		if errors.As(err, &failure) {
			if validateErr := failure.Validate(input.Action, input.Expected); validateErr != nil {
				writeWireError(writer, http.StatusInternalServerError, "invalid_failure", "Environment returned invalid failure evidence.")
				return
			}
			writeWire(writer, http.StatusConflict, wireResponse{Protocol: ProtocolVersionV1, Failure: &failure})
			return
		}
		writeWireError(writer, http.StatusInternalServerError, "apply_failed", "Environment action failed without a public transition.")
		return
	}
	if err := transition.ValidateApply(input.Episode, input.Expected, input.Action); err != nil {
		writeWireError(writer, http.StatusInternalServerError, "invalid_transition", "Environment returned an invalid apply transition.")
		return
	}
	writeWire(writer, http.StatusOK, wireResponse{Protocol: ProtocolVersionV1, Transition: &transition})
}

func decodeRequest(request *http.Request, target any) error {
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxRequestBytes+1))
	if err != nil || len(raw) > maxRequestBytes {
		return ErrInvalidWire
	}
	if err := cognition.ValidateExactJSONObject(raw, target, "cognition transport request"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWire, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalidWire
	}
	return nil
}

func writeWireError(writer http.ResponseWriter, status int, code, message string) {
	writeWire(writer, status, wireResponse{
		Protocol: ProtocolVersionV1, Error: &wireError{Code: code, Message: message},
	})
}

func writeWire(writer http.ResponseWriter, status int, response wireResponse) {
	raw, err := json.Marshal(response)
	if err != nil {
		http.Error(writer, "Unable to encode environment response.", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(raw, '\n'))
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
