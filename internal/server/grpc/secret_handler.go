package grpc

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/arsykor/gophkeeper/internal/server/service"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

const chunkSize = 64 * 1024 // 64 KiB per streaming chunk

// SecretHandler implements pb.SecretServiceServer.
type SecretHandler struct {
	pb.UnimplementedSecretServiceServer
	svc *service.Service
}

// NewSecretHandler creates a SecretHandler.
func NewSecretHandler(svc *service.Service) *SecretHandler {
	return &SecretHandler{svc: svc}
}

// ListSecrets returns metadata-only for all caller's secrets.
func (h *SecretHandler) ListSecrets(ctx context.Context, _ *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	userID, err := userIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	secrets, err := h.svc.ListSecrets(userID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	var metas []*pb.SecretMeta
	for _, s := range secrets {
		metas = append(metas, secretToMeta(s))
	}
	return &pb.ListSecretsResponse{Secrets: metas}, nil
}

// GetSecret returns the full decrypted payload for the requested secret.
func (h *SecretHandler) GetSecret(ctx context.Context, req *pb.GetSecretRequest) (*pb.GetSecretResponse, error) {
	userID, err := userIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	sec, err := h.svc.GetSecret(req.Id, userID)
	if err != nil {
		return nil, mapStorageError(err)
	}
	return &pb.GetSecretResponse{Meta: secretToMeta(sec), Payload: sec.Data}, nil
}

// CreateSecret stores a new encrypted secret.
func (h *SecretHandler) CreateSecret(ctx context.Context, req *pb.CreateSecretRequest) (*pb.CreateSecretResponse, error) {
	userID, err := userIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	sec, err := h.svc.CreateSecret(userID, req.Name, req.Type.String(), req.Metadata, req.Payload)
	if err != nil {
		return nil, mapStorageError(err)
	}
	return &pb.CreateSecretResponse{Meta: secretToMeta(sec)}, nil
}

// UpdateSecret replaces an existing secret with optimistic locking.
func (h *SecretHandler) UpdateSecret(ctx context.Context, req *pb.UpdateSecretRequest) (*pb.UpdateSecretResponse, error) {
	userID, err := userIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	sec, err := h.svc.UpdateSecret(req.Id, userID, req.Name, req.Metadata, req.Payload, req.Version)
	if err != nil {
		if errors.Is(err, postgres.ErrVersionConflict) {
			return nil, status.Error(codes.Aborted, "version conflict: reload and retry")
		}
		return nil, mapStorageError(err)
	}
	return &pb.UpdateSecretResponse{Meta: secretToMeta(sec)}, nil
}

// DeleteSecret permanently removes a secret.
func (h *SecretHandler) DeleteSecret(ctx context.Context, req *pb.DeleteSecretRequest) (*pb.DeleteSecretResponse, error) {
	userID, err := userIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.DeleteSecret(ctx, req.Id, userID); err != nil {
		return nil, mapStorageError(err)
	}
	return &pb.DeleteSecretResponse{}, nil
}

// UploadFile receives a binary file via client-side streaming and stores it in MinIO.
func (h *SecretHandler) UploadFile(stream pb.SecretService_UploadFileServer) error {
	userID, err := userIDFromCtx(stream.Context())
	if err != nil {
		return err
	}

	// First message must be the header.
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Error(codes.InvalidArgument, "expected header as first message")
	}
	hdr, ok := firstMsg.Data.(*pb.UploadFileRequest_Header)
	if !ok {
		return status.Error(codes.InvalidArgument, "first message must be header")
	}

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		for {
			msg, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				errCh <- nil
				return
			}
			if err != nil {
				pw.CloseWithError(err)
				errCh <- err
				return
			}
			chunk, ok := msg.Data.(*pb.UploadFileRequest_Chunk)
			if !ok {
				continue
			}
			if _, err := pw.Write(chunk.Chunk); err != nil {
				pw.CloseWithError(err)
				errCh <- err
				return
			}
		}
	}()

	sec, err := h.svc.UploadFile(stream.Context(), userID, hdr.Header.Name, hdr.Header.Metadata, pr, -1)
	if recvErr := <-errCh; recvErr != nil && err == nil {
		err = recvErr
	}
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	return stream.SendAndClose(&pb.UploadFileResponse{Meta: secretToMeta(sec)})
}

// DownloadFile streams a binary file from MinIO to the client.
func (h *SecretHandler) DownloadFile(req *pb.DownloadFileRequest, stream pb.SecretService_DownloadFileServer) error {
	userID, err := userIDFromCtx(stream.Context())
	if err != nil {
		return err
	}
	sec, rc, err := h.svc.DownloadFile(stream.Context(), req.Id, userID)
	if err != nil {
		return mapStorageError(err)
	}
	defer rc.Close()

	// Send metadata first.
	if err := stream.Send(&pb.DownloadFileResponse{Data: &pb.DownloadFileResponse_Meta{Meta: secretToMeta(sec)}}); err != nil {
		return err
	}

	buf := make([]byte, chunkSize)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if sendErr := stream.Send(&pb.DownloadFileResponse{Data: &pb.DownloadFileResponse_Chunk{Chunk: chunk}}); sendErr != nil {
				return sendErr
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
	return nil
}

func secretToMeta(s *postgres.Secret) *pb.SecretMeta {
	return &pb.SecretMeta{
		Id:        s.ID,
		Name:      s.Name,
		Type:      pbSecretType(s.Type),
		Metadata:  s.Metadata,
		FileSize:  s.FileSize,
		Version:   s.Version,
		CreatedAt: timestamppb.New(s.CreatedAt),
		UpdatedAt: timestamppb.New(s.UpdatedAt),
	}
}

func pbSecretType(t string) pb.SecretType {
	switch t {
	case "SECRET_TYPE_CREDENTIAL":
		return pb.SecretType_SECRET_TYPE_CREDENTIAL
	case "SECRET_TYPE_TEXT":
		return pb.SecretType_SECRET_TYPE_TEXT
	case "SECRET_TYPE_BINARY":
		return pb.SecretType_SECRET_TYPE_BINARY
	case "SECRET_TYPE_CARD":
		return pb.SecretType_SECRET_TYPE_CARD
	default:
		return pb.SecretType_SECRET_TYPE_UNSPECIFIED
	}
}

func mapStorageError(err error) error {
	if errors.Is(err, postgres.ErrNotFound) {
		return status.Error(codes.NotFound, "secret not found")
	}
	if errors.Is(err, postgres.ErrVersionConflict) {
		return status.Error(codes.Aborted, "version conflict: reload and retry")
	}
	if errors.Is(err, postgres.ErrInvalidMetadata) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
