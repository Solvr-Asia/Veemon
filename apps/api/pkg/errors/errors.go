// Package errors defines typed application errors with HTTP/gRPC mappings.
package errors

import (
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AppError struct {
	HTTPStatus int
	GRPCCode   codes.Code
	Code       int
	Message    string
	Err        error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func (e *AppError) GRPCStatus() *status.Status {
	return status.New(e.GRPCCode, e.Message)
}

func (e *AppError) FiberError(c fiber.Ctx) error {
	return c.Status(e.HTTPStatus).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    e.Code,
			"message": e.Message,
		},
	})
}

func New(httpStatus int, grpcCode codes.Code, code int, message string) *AppError {
	return &AppError{
		HTTPStatus: httpStatus,
		GRPCCode:   grpcCode,
		Code:       code,
		Message:    message,
	}
}

func Wrap(err error, httpStatus int, grpcCode codes.Code, code int, message string) *AppError {
	return &AppError{
		HTTPStatus: httpStatus,
		GRPCCode:   grpcCode,
		Code:       code,
		Message:    message,
		Err:        err,
	}
}

func BadRequest(code int, message string) *AppError {
	return New(http.StatusBadRequest, codes.InvalidArgument, code, message)
}

func Unauthorized(message string) *AppError {
	return New(http.StatusUnauthorized, codes.Unauthenticated, 401, message)
}

func Forbidden(message string) *AppError {
	return New(http.StatusForbidden, codes.PermissionDenied, 403, message)
}

func NotFound(message string) *AppError {
	return New(http.StatusNotFound, codes.NotFound, 404, message)
}

func Conflict(code int, message string) *AppError {
	return New(http.StatusConflict, codes.AlreadyExists, code, message)
}

func TooManyRequests(message string) *AppError {
	return New(http.StatusTooManyRequests, codes.ResourceExhausted, 429, message)
}

func Internal(code int, message string) *AppError {
	return New(http.StatusInternalServerError, codes.Internal, code, message)
}

func ValidationError(message string) *AppError {
	return New(http.StatusBadRequest, codes.InvalidArgument, 400, message)
}
