package httpapi

import (
	"net/http"
)

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Details   map[string]string `json:"details"`
}

func (api *API) errorResponse(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]string) {
	if details == nil {
		details = map[string]string{}
	}

	payload := envelope{"error": errorBody{
		Code:      code,
		Message:   message,
		RequestID: RequestID(r.Context()),
		Details:   details,
	}}

	if err := writeJSON(w, status, payload, nil); err != nil {
		api.logger.Error("write error response", "request_id", RequestID(r.Context()), "error", err)
	}
}

func (api *API) internalErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	api.logger.Error("internal server error", "request_id", RequestID(r.Context()), "error", err)
	api.errorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The server encountered an unexpected error.", nil)
}

func (api *API) notFoundResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "The requested resource was not found.", nil)
}

func (api *API) methodNotAllowedResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The request method is not supported for this resource.", nil)
}

func (api *API) badRequestResponse(w http.ResponseWriter, r *http.Request, message string) {
	api.errorResponse(w, r, http.StatusBadRequest, "BAD_REQUEST", message, nil)
}

func (api *API) validationFailedResponse(w http.ResponseWriter, r *http.Request, details map[string]string) {
	api.errorResponse(w, r, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid input provided.", details)
}

func (api *API) authenticationRequiredResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "You must be signed in to do that.", nil)
}

func (api *API) invalidCredentialsResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Invalid email or password.", nil)
}

func (api *API) permissionDeniedResponse(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "You do not have permission to do that."
	}
	api.errorResponse(w, r, http.StatusForbidden, "PERMISSION_DENIED", message, nil)
}

func (api *API) conflictResponse(w http.ResponseWriter, r *http.Request, message string) {
	api.errorResponse(w, r, http.StatusConflict, "CONFLICT", message, nil)
}

func (api *API) rateLimitedResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many attempts. Please try again shortly.", nil)
}

func (api *API) invalidSignupCodeResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusUnprocessableEntity, "INVALID_CODE", "That code is invalid or has expired. Request a new one.", nil)
}

func (api *API) emailAlreadyRegisteredResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "This email is already registered. Sign in instead.", nil)
}

func (api *API) phoneAlreadyRegisteredResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusConflict, "PHONE_ALREADY_REGISTERED", "This phone number is already registered. Sign in instead.", nil)
}

func (api *API) googleSignInFailedResponse(w http.ResponseWriter, r *http.Request) {
	api.errorResponse(w, r, http.StatusUnauthorized, "GOOGLE_SIGNIN_FAILED", "Couldn't sign in with Google. Please try again.", nil)
}
