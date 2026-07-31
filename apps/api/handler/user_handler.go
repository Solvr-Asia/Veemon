// Package handler implements the gRPC/HTTP request handlers.
package handler

import (
	"context"
	"time"

	"veemon-common/monitoring/metrics"
	"veemon/app/usecase/user"
	pb "veemon/handler/grpc/user"
	"veemon/pkg/authguard"
	"veemon/pkg/errors"
	"veemon/pkg/middleware"
	"veemon/pkg/token"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type userHandler struct {
	pb.UnimplementedUserApiServer
	userUC       user.UseCase
	tokenService *token.TokenService
	guard        *authguard.Guard
	logger       *zap.Logger
}

func NewUserHandler(userUC user.UseCase, tokenService *token.TokenService, guard *authguard.Guard, logger *zap.Logger) pb.UserApiServer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &userHandler{
		userUC:       userUC,
		tokenService: tokenService,
		guard:        guard,
		logger:       logger,
	}
}

// internal logs the underlying cause of a 5xx (which is never sent to clients)
// and returns a sanitized error carrying only a stable code and public message.
func (h *userHandler) internal(code int, publicMsg string, cause error) error {
	h.logger.Error("internal handler error",
		zap.Int("code", code),
		zap.String("message", publicMsg),
		zap.Error(cause),
	)
	return errors.Internal(code, publicMsg)
}

// Register creates a new user account.
func (h *userHandler) Register(ctx context.Context, req *pb.RegisterReq) (*pb.RegisterRes, error) {
	if err := pb.ValidateRequest(req); err != nil {
		return nil, err
	}

	result, err := h.userUC.Register(ctx, user.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
		Phone:    req.Phone,
	})
	if err != nil {
		if err == user.ErrEmailExists {
			return nil, errors.Conflict(40901, "email already registered")
		}
		return nil, h.internal(50001, "failed to register user", err)
	}

	if m := metrics.Get(); m != nil {
		m.RecordUserRegistered()
	}

	return &pb.RegisterRes{
		Id:    result.ID,
		Email: result.Email,
		Name:  result.Name,
	}, nil
}

// Login authenticates a user and returns a PASETO token.
func (h *userHandler) Login(ctx context.Context, req *pb.LoginReq) (*pb.LoginRes, error) {
	if err := pb.ValidateRequest(req); err != nil {
		return nil, err
	}

	// Reject early if the account is locked out from repeated failures.
	if h.guard.IsLocked(ctx, req.Email) {
		return nil, errors.TooManyRequests("too many failed login attempts; try again later")
	}

	userEntity, err := h.userUC.Login(ctx, req.Email, req.Password)
	if err != nil {
		switch {
		case err == user.ErrInvalidCreds:
			h.guard.RecordFailure(ctx, req.Email)
			return nil, errors.Unauthorized("invalid email or password")
		case err == user.ErrUserNotActive:
			return nil, errors.Forbidden("account is not active")
		default:
			return nil, h.internal(50002, "failed to login", err)
		}
	}

	// Successful login clears any accumulated failure/lock state.
	h.guard.Reset(ctx, req.Email)

	if m := metrics.Get(); m != nil {
		m.RecordUserLogin()
	}

	// Generate PASETO token
	accessToken, err := h.tokenService.GenerateToken(
		userEntity.ID,
		userEntity.Email,
		userEntity.Roles,
		userEntity.CompanyCode,
	)
	if err != nil {
		return nil, h.internal(50003, "failed to generate token", err)
	}

	return &pb.LoginRes{
		Token: accessToken,
		User: &pb.UserProfile{
			Id:        userEntity.ID,
			Email:     userEntity.Email,
			Name:      userEntity.Name,
			Phone:     userEntity.Phone,
			Status:    string(userEntity.Status),
			CreatedAt: userEntity.CreatedAt.Format(time.RFC3339),
		},
	}, nil
}

// RefreshToken rotates a valid token: it reloads the user from the database so
// role/status changes take effect, revokes the presented token, and issues a
// new one. Note: because the refresh route itself requires a non-expired token,
// this cannot refresh an already-expired token — a separate long-lived refresh
// token (a proto change) would be needed for that.
func (h *userHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenReq) (*pb.RefreshTokenRes, error) {
	authCtx := getAuthFromContext(ctx)
	if authCtx == nil {
		return nil, errors.Unauthorized("authentication required")
	}

	// Reload the user so a deactivated or deleted account cannot keep
	// refreshing, and so fresh roles are embedded in the new token.
	profile, err := h.userUC.GetProfile(ctx, authCtx.UserID)
	if err != nil {
		if err == user.ErrNotFound {
			return nil, errors.Unauthorized("user no longer exists")
		}
		return nil, h.internal(50010, "failed to refresh token", err)
	}
	if string(profile.Status) != "active" {
		return nil, errors.Forbidden("account is not active")
	}

	// Rotate: revoke the presented token so it cannot be reused.
	if authCtx.TokenID != "" {
		_ = h.guard.Revoke(ctx, authCtx.TokenID, time.Until(authCtx.ExpiresAt))
	}

	newToken, err := h.tokenService.GenerateToken(
		profile.ID,
		profile.Email,
		profile.Roles,
		profile.CompanyCode,
	)
	if err != nil {
		return nil, h.internal(50010, "failed to refresh token", err)
	}

	return &pb.RefreshTokenRes{
		Token: newToken,
	}, nil
}

// GetMe returns the profile of the currently authenticated user.
func (h *userHandler) GetMe(ctx context.Context, req *emptypb.Empty) (*pb.UserProfile, error) {
	authCtx := getAuthFromContext(ctx)
	if authCtx == nil {
		return nil, errors.Unauthorized("authentication required")
	}

	profile, err := h.userUC.GetProfile(ctx, authCtx.UserID)
	if err != nil {
		if err == user.ErrNotFound {
			return nil, errors.NotFound("user not found")
		}
		return nil, h.internal(50004, "failed to get profile", err)
	}

	return &pb.UserProfile{
		Id:        profile.ID,
		Email:     profile.Email,
		Name:      profile.Name,
		Phone:     profile.Phone,
		Status:    string(profile.Status),
		CreatedAt: profile.CreatedAt.Format(time.RFC3339),
	}, nil
}

// Logout revokes the presented token so it can no longer be used, even before
// its natural expiry. Requires a Redis-backed guard; without Redis it degrades
// to a client-side-only logout.
func (h *userHandler) Logout(ctx context.Context, req *emptypb.Empty) (*pb.LogoutRes, error) {
	authCtx := getAuthFromContext(ctx)
	if authCtx == nil {
		return nil, errors.Unauthorized("authentication required")
	}

	if authCtx.TokenID != "" {
		_ = h.guard.Revoke(ctx, authCtx.TokenID, time.Until(authCtx.ExpiresAt))
	}

	return &pb.LogoutRes{
		Message: "successfully logged out",
	}, nil
}

// ListUsers returns a paginated list of all users (admin only).
func (h *userHandler) ListUsers(ctx context.Context, req *pb.ListUsersReq) (*pb.ListUsersRes, error) {
	if err := pb.ValidateRequest(req); err != nil {
		return nil, err
	}

	users, total, err := h.userUC.ListAll(ctx, user.ListInput{
		Page:      int(req.Page),
		Size:      int(req.Size),
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		return nil, h.internal(50005, "failed to list users", err)
	}

	pbUsers := make([]*pb.UserProfile, len(users))
	for i, u := range users {
		pbUsers[i] = &pb.UserProfile{
			Id:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			Phone:     u.Phone,
			Status:    string(u.Status),
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
		}
	}

	totalPages := (total + int64(req.Size) - 1) / int64(req.Size)

	return &pb.ListUsersRes{
		Users: pbUsers,
		Pagination: &pb.Pagination{
			Page:       req.Page,
			Size:       req.Size,
			Total:      total,
			TotalPages: int32(totalPages), // #nosec G115 -- totalPages is bounded by pagination
		},
	}, nil
}

// GetUser returns a single user by ID (admin only).
func (h *userHandler) GetUser(ctx context.Context, req *pb.GetUserReq) (*pb.UserProfile, error) {
	if err := validateUserID(req.Id); err != nil {
		return nil, err
	}

	userEntity, err := h.userUC.GetUser(ctx, req.Id)
	if err != nil {
		if err == user.ErrNotFound {
			return nil, errors.NotFound("user not found")
		}
		return nil, h.internal(50006, "failed to get user", err)
	}

	return &pb.UserProfile{
		Id:        userEntity.ID,
		Email:     userEntity.Email,
		Name:      userEntity.Name,
		Phone:     userEntity.Phone,
		Status:    string(userEntity.Status),
		CreatedAt: userEntity.CreatedAt.Format(time.RFC3339),
	}, nil
}

// UpdateUser updates a user by ID (admin only).
func (h *userHandler) UpdateUser(ctx context.Context, req *pb.UpdateUserReq) (*pb.UserProfile, error) {
	if err := validateUserID(req.Id); err != nil {
		return nil, err
	}
	if err := pb.ValidateRequest(req); err != nil {
		return nil, err
	}

	userEntity, err := h.userUC.UpdateUser(ctx, req.Id, user.UpdateInput{
		Name:   req.Name,
		Phone:  req.Phone,
		Status: req.Status,
	})
	if err != nil {
		if err == user.ErrNotFound {
			return nil, errors.NotFound("user not found")
		}
		return nil, h.internal(50007, "failed to update user", err)
	}

	return &pb.UserProfile{
		Id:        userEntity.ID,
		Email:     userEntity.Email,
		Name:      userEntity.Name,
		Phone:     userEntity.Phone,
		Status:    string(userEntity.Status),
		CreatedAt: userEntity.CreatedAt.Format(time.RFC3339),
	}, nil
}

// DeleteUser soft-deletes a user by ID (admin only).
func (h *userHandler) DeleteUser(ctx context.Context, req *pb.DeleteUserReq) (*pb.DeleteUserRes, error) {
	if err := validateUserID(req.Id); err != nil {
		return nil, err
	}

	err := h.userUC.DeleteUser(ctx, req.Id)
	if err != nil {
		if err == user.ErrNotFound {
			return nil, errors.NotFound("user not found")
		}
		return nil, h.internal(50008, "failed to delete user", err)
	}

	return &pb.DeleteUserRes{
		Message: "user deleted successfully",
	}, nil
}

func getAuthFromContext(ctx context.Context) *middleware.AuthContext {
	a, _ := middleware.AuthFromContext(ctx)
	return a
}

// validateUserID rejects malformed identifiers before they reach the database,
// where an invalid UUID would surface as a 500 instead of a 400.
func validateUserID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return errors.BadRequest(40002, "invalid user id")
	}
	return nil
}
