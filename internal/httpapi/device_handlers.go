package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type enrollDeviceRequest struct {
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name"`
}

type deviceResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	EnrolledAt  string `json:"enrolled_at"`
}

// enrollDevice implements POST /api/v1/devices/enroll. Any active member
// may enroll a device for themselves — the resulting device's user_id is
// always the caller, never a value from the request body. Seeing or
// revoking the full device roster (list/revoke below) is a separate,
// owner-only capability (devices:manage): enrolling your own phone and
// administering everyone else's are different things.
func (api *API) enrollDevice(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("enrollDevice ran without requireAuth"))
		return
	}
	business, ok := tenancy.BusinessFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("enrollDevice ran without requireBusinessContext"))
		return
	}

	var req enrollDeviceRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	v.Check(req.PublicKey != "", "public_key", "must be provided")
	v.Check(req.DisplayName != "", "display_name", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	location, err := api.identity.GetDefaultLocation(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("load default location: %w", err))
		return
	}

	device, err := api.identity.CreateDevice(r.Context(), principal.UserID, business.BusinessID, location.ID, req.PublicKey, req.DisplayName)
	if err != nil {
		if errors.Is(err, identity.ErrDevicePublicKeyTaken) {
			api.conflictResponse(w, r, "This device is already enrolled.")
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("enroll device: %w", err))
		return
	}

	lease, err := api.issueOfflineLease(device, business.BusinessID, principal.UserID, location.ID, business.Role)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("issue offline lease: %w", err))
		return
	}

	payload := envelope{"data": envelope{
		"device": toDeviceResponse(device),
		"lease":  lease,
	}}
	if err := writeJSON(w, http.StatusCreated, payload, nil); err != nil {
		api.logger.Error("write enroll device response", "request_id", RequestID(r.Context()), "error", err)
	}
}

func (api *API) issueOfflineLease(device identity.Device, businessID, userID, locationID uuid.UUID, role string) (string, error) {
	if api.offlineSigningPrivateKey == nil {
		return "", errors.New("offline lease signing is not configured")
	}

	now := time.Now().UTC()
	capabilities := tenancy.OfflineSafe(role).List()
	names := make([]string, len(capabilities))
	for i, c := range capabilities {
		names[i] = string(c)
	}

	return auth.IssueLease(api.offlineSigningPrivateKey, auth.LeasePayload{
		BusinessID:   businessID,
		UserID:       userID,
		DeviceID:     device.ID,
		LocationID:   locationID,
		Role:         role,
		Capabilities: names,
		IssuedAt:     now,
		ExpiresAt:    now.Add(api.offlineLeaseTTL),
	})
}

// listDevices implements GET /api/v1/devices. Owner-only (devices:manage):
// seeing every enrolled device across the business is device
// administration, not something a Staff member's own enrollment implies.
func (api *API) listDevices(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listDevices ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityDevicesManage)
	if !ok {
		return
	}

	devices, err := api.identity.ListDevices(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list devices: %w", err))
		return
	}

	out := make([]deviceResponse, 0, len(devices))
	for _, d := range devices {
		out = append(out, toDeviceResponse(d))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list devices response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// revokeDevice implements DELETE /api/v1/devices/{device_id}. Owner-only
// (devices:manage), same reasoning as listDevices.
func (api *API) revokeDevice(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("revokeDevice ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityDevicesManage)
	if !ok {
		return
	}

	deviceID, err := uuid.Parse(chi.URLParam(r, "device_id"))
	if err != nil {
		api.badRequestResponse(w, r, "device_id must be a valid UUID.")
		return
	}

	if _, err := api.identity.RevokeDevice(r.Context(), principal.UserID, business.BusinessID, deviceID); err != nil {
		if errors.Is(err, identity.ErrDeviceNotFound) {
			api.notFoundResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("revoke device: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toDeviceResponse(d identity.Device) deviceResponse {
	return deviceResponse{
		ID:          d.ID.String(),
		DisplayName: d.DisplayName,
		Status:      d.Status,
		EnrolledAt:  d.EnrolledAt.UTC().Format(time.RFC3339),
	}
}
