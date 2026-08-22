package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Code represents an API error code string defined in docs/api-v1.md §0.
type Code string

const (
	// 400 Bad Request
	CodeValidationError    Code = "validation_error"
	CodeFileTooLarge       Code = "file_too_large"
	CodeInvalidToken       Code = "invalid_token"
	CodeInvalidCredentials Code = "invalid_credentials"

	// 401 Unauthorized
	CodeUnauthenticated Code = "unauthenticated"

	// 403 Forbidden
	CodeForbidden       Code = "forbidden"
	CodeGuestReadOnly   Code = "guest_read_only"
	CodeAccountDisabled Code = "account_disabled"

	// 404 Not Found
	CodeNotFound    Code = "not_found"
	CodeInvalidCode Code = "invalid_code"

	// 409 Conflict
	CodeEmailAlreadyExists Code = "email_already_exists"
	CodeDeviceLimitReached Code = "device_limit_reached"
	CodeAlreadyMember      Code = "already_member"
	CodeAlreadyClaimed     Code = "already_claimed"
	CodeAttemptsExhausted  Code = "attempts_exhausted"
	CodeDeadlinePassed     Code = "deadline_passed"
	CodeSetupAlreadyDone   Code = "setup_already_done"
	CodeAttemptConflict    Code = "attempt_conflict"

	// 410 Gone
	CodeTokenUsed    Code = "token_used"
	CodeTokenExpired Code = "token_expired"

	// 429 Too Many Requests
	CodeRateLimited Code = "rate_limited"

	// 503 Service Unavailable
	CodeCapacityUnavailable Code = "capacity_unavailable"
)

// ErrorSpec defines the HTTP status and default message for an error code.
type ErrorSpec struct {
	Code       Code
	HTTPStatus int
	Message    string
}

// Table maps each error code to its standard HTTP status and default message.
var Table = map[Code]ErrorSpec{
	CodeValidationError: {
		Code:       CodeValidationError,
		HTTPStatus: http.StatusBadRequest,
		Message:    "Body malformado o campo inválido",
	},
	CodeFileTooLarge: {
		Code:       CodeFileTooLarge,
		HTTPStatus: http.StatusBadRequest,
		Message:    "Archivo supera el límite permitido (10 MB por archivo)",
	},
	CodeInvalidToken: {
		Code:       CodeInvalidToken,
		HTTPStatus: http.StatusBadRequest,
		Message:    "Token malformado o inválido",
	},
	CodeInvalidCredentials: {
		Code:       CodeInvalidCredentials,
		HTTPStatus: http.StatusUnauthorized,
		Message:    "Email o contraseña incorrectos",
	},
	CodeUnauthenticated: {
		Code:       CodeUnauthenticated,
		HTTPStatus: http.StatusUnauthorized,
		Message:    "Sin sesión o sesión expirada",
	},
	CodeForbidden: {
		Code:       CodeForbidden,
		HTTPStatus: http.StatusForbidden,
		Message:    "No tenés permiso para acceder a este recurso",
	},
	CodeGuestReadOnly: {
		Code:       CodeGuestReadOnly,
		HTTPStatus: http.StatusForbidden,
		Message:    "Los invitados tienen acceso de solo lectura",
	},
	CodeAccountDisabled: {
		Code:       CodeAccountDisabled,
		HTTPStatus: http.StatusForbidden,
		Message:    "Tu cuenta está desactivada; contactate con la dirección de la escuela",
	},
	CodeNotFound: {
		Code:       CodeNotFound,
		HTTPStatus: http.StatusNotFound,
		Message:    "Recurso inexistente",
	},
	CodeInvalidCode: {
		Code:       CodeInvalidCode,
		HTTPStatus: http.StatusNotFound,
		Message:    "Este código ya no funciona; pedile el nuevo a tu docente",
	},
	CodeEmailAlreadyExists: {
		Code:       CodeEmailAlreadyExists,
		HTTPStatus: http.StatusConflict,
		Message:    "Ya existe una cuenta con este email",
	},
	CodeDeviceLimitReached: {
		Code:       CodeDeviceLimitReached,
		HTTPStatus: http.StatusConflict,
		Message:    "Se alcanzó el límite máximo de dispositivos vinculados",
	},
	CodeAlreadyMember: {
		Code:       CodeAlreadyMember,
		HTTPStatus: http.StatusConflict,
		Message:    "Ya sos miembro de esta aula",
	},
	CodeAlreadyClaimed: {
		Code:       CodeAlreadyClaimed,
		HTTPStatus: http.StatusConflict,
		Message:    "Este código o sesión ya fue reclamada",
	},
	CodeAttemptsExhausted: {
		Code:       CodeAttemptsExhausted,
		HTTPStatus: http.StatusConflict,
		Message:    "No te quedan intentos disponibles para esta consigna",
	},
	CodeDeadlinePassed: {
		Code:       CodeDeadlinePassed,
		HTTPStatus: http.StatusConflict,
		Message:    "El plazo de entrega ha vencido y no se admiten entregas tardías",
	},
	CodeSetupAlreadyDone: {
		Code:       CodeSetupAlreadyDone,
		HTTPStatus: http.StatusConflict,
		Message:    "La configuración inicial de la escuela ya fue realizada",
	},
	CodeAttemptConflict: {
		Code:       CodeAttemptConflict,
		HTTPStatus: http.StatusConflict,
		Message:    "Conflicto con el número de intento; por favor actualizá la vista",
	},
	CodeTokenUsed: {
		Code:       CodeTokenUsed,
		HTTPStatus: http.StatusGone,
		Message:    "Este token o enlace ya fue utilizado",
	},
	CodeTokenExpired: {
		Code:       CodeTokenExpired,
		HTTPStatus: http.StatusGone,
		Message:    "Este token o enlace ha expirado",
	},
	CodeRateLimited: {
		Code:       CodeRateLimited,
		HTTPStatus: http.StatusTooManyRequests,
		Message:    "Demasiadas solicitudes; por favor intentá más tarde",
	},
	CodeCapacityUnavailable: {
		Code:       CodeCapacityUnavailable,
		HTTPStatus: http.StatusServiceUnavailable,
		Message:    "Capacidad temporalmente agotada en el runner; por favor reintentá en unos minutos",
	},
}

// APIError represents the error structure as serialized according to docs/api-v1.md §0:
// {"error": {"code": "...", "message": "..."}}
type APIError struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s (HTTP %d)", e.Code, e.Message, e.HTTPStatus)
}

// ErrorResponse wraps APIError in the top-level "error" key.
type ErrorResponse struct {
	Error *APIError `json:"error"`
}

// New creates an APIError for the given code using the default table spec.
// Optional custom messages can override the default.
func New(code Code, customMsg ...string) *APIError {
	spec, ok := Table[code]
	if !ok {
		return &APIError{
			Code:       code,
			Message:    "Error inesperado",
			HTTPStatus: http.StatusInternalServerError,
		}
	}

	msg := spec.Message
	if len(customMsg) > 0 && customMsg[0] != "" {
		msg = customMsg[0]
	}

	return &APIError{
		Code:       spec.Code,
		Message:    msg,
		HTTPStatus: spec.HTTPStatus,
	}
}

// Write writes the API error response as JSON with appropriate HTTP status code.
func Write(w http.ResponseWriter, apiErr *APIError) {
	if apiErr == nil {
		apiErr = &APIError{
			Code:       "internal_error",
			Message:    "Error interno del servidor",
			HTTPStatus: http.StatusInternalServerError,
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: apiErr})
}

// WriteCode is a convenience helper to create and write an APIError in one call.
func WriteCode(w http.ResponseWriter, code Code, customMsg ...string) {
	Write(w, New(code, customMsg...))
}
