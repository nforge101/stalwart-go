package stalwart

import (
	"errors"
	"fmt"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{"without message", &APIError{StatusCode: 404}, "stalwart: HTTP 404"},
		{"with message", &APIError{StatusCode: 500, Message: "boom"}, "stalwart: HTTP 500: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{400, nil},
		{401, ErrUnauthorized},
		{403, ErrForbidden},
		{404, nil},
		{500, nil},
	}
	for _, tt := range tests {
		got := (&APIError{StatusCode: tt.status}).Unwrap()
		if got != tt.want {
			t.Errorf("status %d: Unwrap() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestMethodError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *MethodError
		want string
	}{
		{
			name: "without description",
			err:  &MethodError{Type: "forbidden"},
			want: "stalwart: method error forbidden",
		},
		{
			name: "with description",
			err:  &MethodError{Type: "forbidden", Description: "no access"},
			want: "stalwart: method error forbidden: no access",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMethodError_Unwrap(t *testing.T) {
	tests := []struct {
		typ  string
		want error
	}{
		{"forbidden", ErrForbidden},
		{"invalidArguments", nil},
		{"unknownMethod", nil},
		{"", nil},
	}
	for _, tt := range tests {
		got := (&MethodError{Type: tt.typ}).Unwrap()
		if got != tt.want {
			t.Errorf("type %q: Unwrap() = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

// TestErrorsMatchWhenWrapped checks the pattern callers use: wrapping an error
// with %w keeps both errors.As and errors.Is working.
func TestErrorsMatchWhenWrapped(t *testing.T) {
	wrapped := fmt.Errorf("delete domain: %w", &MethodError{Type: "forbidden"})

	var methodErr *MethodError
	if !errors.As(wrapped, &methodErr) {
		t.Fatal("errors.As did not find *MethodError through the wrapper")
	}
	if methodErr.Type != "forbidden" {
		t.Errorf("Type = %q, want forbidden", methodErr.Type)
	}
	if !errors.Is(wrapped, ErrForbidden) {
		t.Error("errors.Is did not find ErrForbidden through the wrapper")
	}
}

// TestSentinelsAreDistinct checks that no sentinel matches another, so callers
// can rely on errors.Is for each category.
func TestSentinelsAreDistinct(t *testing.T) {
	sentinels := []error{ErrInvalidConfig, ErrUnauthorized, ErrForbidden, ErrTransport, ErrDecode, ErrNotFound, ErrAlreadyExists}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v matches %v", a, b)
			}
		}
	}
}
