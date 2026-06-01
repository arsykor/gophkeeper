// Package grpc provides a typed wrapper around the GophKeeper gRPC clients.
package grpc

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// Client wraps the two gRPC service clients and manages the connection.
type Client struct {
	conn   *grpc.ClientConn
	auth   pb.AuthServiceClient
	secret pb.SecretServiceClient
	token  string
}

// New dials the server and returns a Client. Call Close() when done.
func New(address string, insecureDial bool) (*Client, error) {
	var opts []grpc.DialOption
	if insecureDial {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		auth:   pb.NewAuthServiceClient(conn),
		secret: pb.NewSecretServiceClient(conn),
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// SetToken stores the JWT token for subsequent authenticated calls.
func (c *Client) SetToken(token string) {
	c.token = token
}

// Token returns the current stored JWT token.
func (c *Client) Token() string {
	return c.token
}

// IsAuthenticated returns true when a token has been set.
func (c *Client) IsAuthenticated() bool {
	return c.token != ""
}

func (c *Client) authCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", c.token)
}

// Register creates a new user account.
func (c *Client) Register(ctx context.Context, login, password string) (string, error) {
	resp, err := c.auth.Register(ctx, &pb.RegisterRequest{Login: login, Password: password})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// Login authenticates an existing user.
func (c *Client) Login(ctx context.Context, login, password string) (string, error) {
	resp, err := c.auth.Login(ctx, &pb.LoginRequest{Login: login, Password: password})
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// ListSecrets returns metadata for all caller's secrets.
func (c *Client) ListSecrets(ctx context.Context) ([]*pb.SecretMeta, error) {
	resp, err := c.secret.ListSecrets(c.authCtx(ctx), &pb.ListSecretsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Secrets, nil
}

// GetSecret fetches the full payload for a secret.
func (c *Client) GetSecret(ctx context.Context, id string) (*pb.GetSecretResponse, error) {
	return c.secret.GetSecret(c.authCtx(ctx), &pb.GetSecretRequest{Id: id})
}

// CreateSecret stores a new secret and returns its metadata.
func (c *Client) CreateSecret(ctx context.Context, name string, t pb.SecretType, meta string, payload []byte) (*pb.SecretMeta, error) {
	resp, err := c.secret.CreateSecret(c.authCtx(ctx), &pb.CreateSecretRequest{
		Name:     name,
		Type:     t,
		Metadata: meta,
		Payload:  payload,
	})
	if err != nil {
		return nil, err
	}
	return resp.Meta, nil
}

// UpdateSecret replaces an existing secret. version must match the current value.
func (c *Client) UpdateSecret(ctx context.Context, id, name, meta string, payload []byte, version int32) (*pb.SecretMeta, error) {
	resp, err := c.secret.UpdateSecret(c.authCtx(ctx), &pb.UpdateSecretRequest{
		Id:       id,
		Name:     name,
		Metadata: meta,
		Payload:  payload,
		Version:  version,
	})
	if err != nil {
		return nil, err
	}
	return resp.Meta, nil
}

// DeleteSecret removes a secret permanently.
func (c *Client) DeleteSecret(ctx context.Context, id string) error {
	_, err := c.secret.DeleteSecret(c.authCtx(ctx), &pb.DeleteSecretRequest{Id: id})
	return err
}

// UploadFile uploads a binary file using client-side streaming.
func (c *Client) UploadFile(ctx context.Context, name, meta string, r io.Reader) (*pb.SecretMeta, error) {
	stream, err := c.secret.UploadFile(c.authCtx(ctx))
	if err != nil {
		return nil, err
	}

	// Send header.
	if err := stream.Send(&pb.UploadFileRequest{
		Data: &pb.UploadFileRequest_Header{
			Header: &pb.UploadHeader{Name: name, Metadata: meta},
		},
	}); err != nil {
		return nil, err
	}

	// Stream chunks.
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if sendErr := stream.Send(&pb.UploadFileRequest{
				Data: &pb.UploadFileRequest_Chunk{Chunk: chunk},
			}); sendErr != nil {
				return nil, sendErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, err
	}
	return resp.Meta, nil
}

// DownloadFile downloads a binary secret. Returns meta and an in-memory reader.
func (c *Client) DownloadFile(ctx context.Context, id string) (*pb.SecretMeta, io.Reader, error) {
	stream, err := c.secret.DownloadFile(c.authCtx(ctx), &pb.DownloadFileRequest{Id: id})
	if err != nil {
		return nil, nil, err
	}

	var meta *pb.SecretMeta
	buf := &bytesBuffer{}

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		switch d := msg.Data.(type) {
		case *pb.DownloadFileResponse_Meta:
			meta = d.Meta
		case *pb.DownloadFileResponse_Chunk:
			buf.Write(d.Chunk)
		}
	}
	return meta, buf, nil
}

// bytesBuffer is a simple in-memory io.Reader backed by a byte slice.
type bytesBuffer struct {
	data []byte
	pos  int
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}
