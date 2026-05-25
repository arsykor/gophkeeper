// Package grpc implements the gRPC server handlers and middleware.
package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/arsykor/gophkeeper/internal/server/auth"
)

type contextKey string

// UserIDKey is the context key under which the authenticated user ID is stored.
const UserIDKey contextKey = "user_id"

// AuthInterceptor validates the JWT in gRPC metadata and injects the user ID
// into the request context. Public methods listed in publicMethods are skipped.
func AuthInterceptor(authMgr *auth.Manager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		ctx, err := authenticate(ctx, authMgr)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor is the streaming equivalent of AuthInterceptor.
func StreamAuthInterceptor(authMgr *auth.Manager) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if isPublicMethod(info.FullMethod) {
			return handler(srv, ss)
		}
		ctx, err := authenticate(ss.Context(), authMgr)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ss, ctx})
	}
}

func authenticate(ctx context.Context, authMgr *auth.Manager) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization token")
	}
	claims, err := authMgr.ValidateToken(values[0])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return context.WithValue(ctx, UserIDKey, claims.UserID), nil
}

func isPublicMethod(method string) bool {
	public := map[string]bool{
		"/gophkeeper.v1.AuthService/Register": true,
		"/gophkeeper.v1.AuthService/Login":    true,
	}
	return public[method]
}

// userIDFromCtx extracts the authenticated user ID from context.
func userIDFromCtx(ctx context.Context) (string, error) {
	v, ok := ctx.Value(UserIDKey).(string)
	if !ok || v == "" {
		return "", status.Error(codes.Unauthenticated, "unauthenticated")
	}
	return v, nil
}

// wrappedStream wraps grpc.ServerStream with a substituted context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
