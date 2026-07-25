package user

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"veemon/pkg/middleware"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/types/known/emptypb"
)

// stubServer implements UserApiServer with canned responses so we can assert
// that the generated Fiber routes bind params/body, apply auth, and shape
// responses correctly.
type stubServer struct {
	UnimplementedUserApiServer
}

func (stubServer) Register(_ context.Context, req *RegisterReq) (*RegisterRes, error) {
	return &RegisterRes{Id: "new-id", Email: req.Email, Name: req.Name}, nil
}

func (stubServer) GetUser(_ context.Context, req *GetUserReq) (*UserProfile, error) {
	return &UserProfile{Id: req.Id, Email: "u@example.com"}, nil
}

func (stubServer) ListUsers(_ context.Context, req *ListUsersReq) (*ListUsersRes, error) {
	return &ListUsersRes{
		Users:      []*UserProfile{{Id: "1"}, {Id: "2"}},
		Pagination: &Pagination{Page: req.Page, Size: req.Size, Total: 2},
	}, nil
}

func (stubServer) Logout(_ context.Context, _ *emptypb.Empty) (*LogoutRes, error) {
	return &LogoutRes{Message: "bye"}, nil
}

// roleValidator authorizes any request whose bearer token names the roles it
// should carry (comma-separated), e.g. "Bearer admin".
func roleValidator(token string) (*middleware.AuthContext, error) {
	return &middleware.AuthContext{UserID: "uid", Roles: strings.Split(token, ",")}, nil
}

func newTestApp() *fiber.App {
	app := fiber.New()
	RegisterUserApiRoutes(app, stubServer{}, roleValidator)
	return app
}

func doJSON(t *testing.T, app *fiber.App, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// Fiber v3 replaced the millisecond timeout argument with TestConfig; a zero
	// Timeout disables it, matching v2's -1.
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0, FailOnTimeout: false})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func TestGeneratedRoutes_PublicRegisterReturns201AndBindsBody(t *testing.T) {
	app := newTestApp()
	code, out := doJSON(t, app, "POST", "/api/v1/auth/register", "",
		`{"email":"a@b.com","password":"Passw0rd!","name":"Ann"}`)
	if code != fiber.StatusCreated {
		t.Fatalf("want 201, got %d (%v)", code, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["email"] != "a@b.com" || data["name"] != "Ann" {
		t.Fatalf("body not bound into request: %v", out)
	}
}

func TestGeneratedRoutes_ProtectedRouteRejectsMissingToken(t *testing.T) {
	app := newTestApp()
	code, _ := doJSON(t, app, "GET", "/api/v1/users/abc", "", "")
	if code != fiber.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", code)
	}
}

func TestGeneratedRoutes_PathParamBoundAndRoleAllowed(t *testing.T) {
	app := newTestApp()
	code, out := doJSON(t, app, "GET", "/api/v1/users/abc", "admin", "")
	if code != fiber.StatusOK {
		t.Fatalf("want 200 for admin, got %d (%v)", code, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["id"] != "abc" {
		t.Fatalf("path param not bound: %v", out)
	}
}

func TestGeneratedRoutes_RoleEnforced(t *testing.T) {
	app := newTestApp()
	code, _ := doJSON(t, app, "GET", "/api/v1/users/abc", "member", "")
	if code != fiber.StatusForbidden {
		t.Fatalf("want 403 for non-admin, got %d", code)
	}
}

func TestGeneratedRoutes_ListEnvelopeWithQueryAndMeta(t *testing.T) {
	app := newTestApp()
	code, out := doJSON(t, app, "GET", "/api/v1/users?page=3&size=25", "admin", "")
	if code != fiber.StatusOK {
		t.Fatalf("want 200, got %d (%v)", code, out)
	}
	data, ok := out["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("want list of 2 under data, got %v", out["data"])
	}
	meta, _ := out["meta"].(map[string]any)
	if meta == nil || meta["page"].(float64) != 3 || meta["size"].(float64) != 25 {
		t.Fatalf("query params not bound into meta: %v", out["meta"])
	}
}

func TestGeneratedRoutes_EmptyInputMethod(t *testing.T) {
	app := newTestApp()
	code, out := doJSON(t, app, "POST", "/api/v1/auth/logout", "admin", "")
	if code != fiber.StatusOK {
		t.Fatalf("want 200 for logout, got %d (%v)", code, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["message"] != "bye" {
		t.Fatalf("unexpected logout body: %v", out)
	}
}
