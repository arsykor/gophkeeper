package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/arsykor/gophkeeper/internal/server/service"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// AuthHandler implements pb.AuthServiceServer.
type AuthHandler struct {
	pb.UnimplementedAuthServiceServer
	svc *service.Service
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(svc *service.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Register creates a new user account and returns a JWT.
func (h *AuthHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	if req.Login == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "login and password are required")
	}
	token, err := h.svc.Register(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, postgres.ErrLoginTaken) {
			return nil, status.Error(codes.AlreadyExists, "login already taken")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.AuthResponse{Token: token}, nil
}

// Login authenticates an existing user and returns a JWT.
func (h *AuthHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	if req.Login == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "login and password are required")
	}
	token, err := h.svc.Login(req.Login, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.AuthResponse{Token: token}, nil
}
