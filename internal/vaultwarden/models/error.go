package models

import (
	"errors"
	"fmt"
	"strings"
)

// AdminErrorModel represents the error model in the admin API response
type AdminErrorModel struct {
	Message string `json:"message"`
	Object  string `json:"object"`
}

// AdminError represents the complete error response from the Vaultwarden admin API
type AdminError struct {
	Error            string              `json:"error"`
	ErrorModel       *AdminErrorModel    `json:"errorModel"`
	ErrorDescription string              `json:"error_description"`
	ExceptionMessage *string             `json:"exceptionMessage"`
	ExceptionTrace   *string             `json:"exceptionStackTrace"`
	InnerException   *string             `json:"innerExceptionMessage"`
	Message          string              `json:"message"`
	Object           string              `json:"object"`
	ValidationErrors map[string][]string `json:"validationErrors"`
}

// APIErrorDetail represents the error structure in the regular API response
type APIErrorDetail struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// APIError represents the error response from the regular Vaultwarden API
type APIError struct {
	Error APIErrorDetail `json:"error"`
}

// VaultwardenError is a wrapper that can handle both error types
type VaultwardenError struct {
	AdminError *AdminError
	APIError   *APIError
	Path       string // Store the path to determine which error type to use
}

func (e *VaultwardenError) Error() string {
	if strings.HasPrefix(e.Path, "/admin") {
		return e.formatAdminError()
	}
	return e.formatAPIError()
}

func (e *VaultwardenError) formatAdminError() string {
	if e.AdminError == nil {
		return "unknown error occurred"
	}

	var messages []string

	// Add the main message if present
	if e.AdminError.Message != "" {
		messages = append(messages, e.AdminError.Message)
	}

	// Add the error model message if different from the main message
	if e.AdminError.ErrorModel != nil && e.AdminError.ErrorModel.Message != "" &&
		e.AdminError.ErrorModel.Message != e.AdminError.Message {
		messages = append(messages, e.AdminError.ErrorModel.Message)
	}

	// Add validation errors if present
	if len(e.AdminError.ValidationErrors) > 0 {
		for field, errs := range e.AdminError.ValidationErrors {
			if field == "" {
				messages = append(messages, errs...)
			} else {
				for _, err := range errs {
					messages = append(messages, fmt.Sprintf("%s: %s", field, err))
				}
			}
		}
	}

	return strings.Join(messages, "; ")
}

func (e *VaultwardenError) formatAPIError() string {
	if e.APIError == nil {
		return "unknown error occurred"
	}

	if e.APIError.Error.Description != "" {
		return fmt.Sprintf("%s (Code: %d, Reason: %s)",
			e.APIError.Error.Description,
			e.APIError.Error.Code,
			e.APIError.Error.Reason)
	}

	return fmt.Sprintf("API error: %d - %s",
		e.APIError.Error.Code,
		e.APIError.Error.Reason)
}

func (e *VaultwardenError) IsNotFound() bool {
	if strings.HasPrefix(e.Path, "/admin") {
		return e.AdminError != nil &&
			(strings.Contains(strings.ToLower(e.AdminError.Message), "doesn't exist") ||
				strings.Contains(strings.ToLower(e.AdminError.Message), "not found"))
	}
	return e.APIError != nil && e.APIError.Error.Code == 404
}

func (e *VaultwardenError) IsValidationError() bool {
	if strings.HasPrefix(e.Path, "/admin") {
		return e.AdminError != nil && len(e.AdminError.ValidationErrors) > 0
	}
	return e.APIError != nil && e.APIError.Error.Code == 400
}

func (e *VaultwardenError) IsAuthenticationError() bool {
	if strings.HasPrefix(e.Path, "/admin") {
		return e.AdminError != nil &&
			(strings.Contains(strings.ToLower(e.AdminError.Message), "unauthorized") ||
				strings.Contains(strings.ToLower(e.AdminError.Message), "invalid token"))
	}
	return e.APIError != nil && e.APIError.Error.Code == 401
}

func IsNotFound(err error) bool {
	var vwErr *VaultwardenError
	if errors.As(err, &vwErr) {
		return vwErr.IsNotFound()
	}
	return false
}

func IsValidationError(err error) bool {
	var vwErr *VaultwardenError
	if errors.As(err, &vwErr) {
		return vwErr.IsValidationError()
	}
	return false
}

func IsAuthenticationError(err error) bool {
	var vwErr *VaultwardenError
	if errors.As(err, &vwErr) {
		return vwErr.IsAuthenticationError()
	}
	return false
}
