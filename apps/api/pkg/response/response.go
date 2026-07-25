// Package response provides standard JSON API response helpers.
package response

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

// protoMarshaler emits the proto's json_name fields (camelCase) so REST output
// matches the .proto contract and the OpenAPI docs, rather than the snake_case
// Go struct tags that encoding/json would use. Unpopulated fields are emitted so
// the response shape is stable.
var protoMarshaler = protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: true}

func marshalProto(msg proto.Message) (json.RawMessage, error) {
	b, err := protoMarshaler.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// SuccessProto writes a proto message as the data payload using protojson.
func SuccessProto(c fiber.Ctx, msg proto.Message) error {
	raw, err := marshalProto(msg)
	if err != nil {
		return InternalError(c, 500, "failed to encode response")
	}
	return c.JSON(Response{Success: true, Data: raw})
}

// CreatedProto is SuccessProto with a 201 status.
func CreatedProto(c fiber.Ctx, msg proto.Message) error {
	raw, err := marshalProto(msg)
	if err != nil {
		return InternalError(c, 500, "failed to encode response")
	}
	return c.Status(fiber.StatusCreated).JSON(Response{Success: true, Data: raw})
}

// SuccessProtoList writes a list of proto messages plus optional proto meta.
func SuccessProtoList(c fiber.Ctx, msgs []proto.Message, meta proto.Message) error {
	items := make([]json.RawMessage, len(msgs))
	for i, m := range msgs {
		raw, err := marshalProto(m)
		if err != nil {
			return InternalError(c, 500, "failed to encode response")
		}
		items[i] = raw
	}

	resp := Response{Success: true, Data: items}
	if meta != nil {
		metaRaw, err := marshalProto(meta)
		if err != nil {
			return InternalError(c, 500, "failed to encode response")
		}
		resp.Meta = metaRaw
	}
	return c.JSON(resp)
}

type ErrorResponse struct {
	Success bool `json:"success"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func Success(c fiber.Ctx, data interface{}) error {
	return c.JSON(Response{
		Success: true,
		Data:    data,
	})
}

func SuccessWithMeta(c fiber.Ctx, data, meta interface{}) error {
	return c.JSON(Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

func Created(c fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(Response{
		Success: true,
		Data:    data,
	})
}

func NoContent(c fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func Error(c fiber.Ctx, status int, code int, message string) error {
	return c.Status(status).JSON(ErrorResponse{
		Success: false,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{
			Code:    code,
			Message: message,
		},
	})
}

func BadRequest(c fiber.Ctx, code int, message string) error {
	return Error(c, fiber.StatusBadRequest, code, message)
}

func Unauthorized(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusUnauthorized, 401, message)
}

func Forbidden(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusForbidden, 403, message)
}

func NotFound(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusNotFound, 404, message)
}

func Conflict(c fiber.Ctx, code int, message string) error {
	return Error(c, fiber.StatusConflict, code, message)
}

func InternalError(c fiber.Ctx, code int, message string) error {
	return Error(c, fiber.StatusInternalServerError, code, message)
}
