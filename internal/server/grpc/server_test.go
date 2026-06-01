package grpc_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/arsykor/gophkeeper/internal/server/auth"
	servergrpc "github.com/arsykor/gophkeeper/internal/server/grpc"
	"github.com/arsykor/gophkeeper/internal/server/service"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

const bufSize = 1024 * 1024
const testEncKey = "0000000000000000000000000000000000000000000000000000000000000000"

func newTestServer(t *testing.T) (*bufconn.Listener, pb.AuthServiceClient, pb.SecretServiceClient) {
	t.Helper()

	authMgr := auth.New("test-jwt-secret")
	enc, err := auth.NewEncryptor(testEncKey)
	require.NoError(t, err)

	users := newMockUsers()
	secrets := newMockSecrets()
	files := &noopFiles{}
	svc := service.New(users, secrets, files, authMgr, enc)

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(servergrpc.AuthInterceptor(authMgr)),
		grpc.StreamInterceptor(servergrpc.StreamAuthInterceptor(authMgr)),
	)
	pb.RegisterAuthServiceServer(srv, servergrpc.NewAuthHandler(svc))
	pb.RegisterSecretServiceServer(srv, servergrpc.NewSecretHandler(svc))

	lis := bufconn.Listen(bufSize)
	t.Cleanup(func() { lis.Close() })

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Server stopped; ignore.
		}
	}()
	t.Cleanup(srv.GracefulStop)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return lis, pb.NewAuthServiceClient(conn), pb.NewSecretServiceClient(conn)
}

func authCtx(t *testing.T, token string) context.Context {
	t.Helper()
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", token)
}

func TestRegisterAndLogin(t *testing.T) {
	_, authClient, _ := newTestServer(t)

	resp, err := authClient.Register(context.Background(), &pb.RegisterRequest{
		Login: "alice", Password: "password",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)

	loginResp, err := authClient.Login(context.Background(), &pb.LoginRequest{
		Login: "alice", Password: "password",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, loginResp.Token)
}

func TestRegister_DuplicateLogin(t *testing.T) {
	_, authClient, _ := newTestServer(t)

	_, err := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "pass"})
	require.NoError(t, err)

	_, err = authClient.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "pass2"})
	assert.Error(t, err)
}

func TestLogin_WrongPassword(t *testing.T) {
	_, authClient, _ := newTestServer(t)

	_, err := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "carol", Password: "right"})
	require.NoError(t, err)

	_, err = authClient.Login(context.Background(), &pb.LoginRequest{Login: "carol", Password: "wrong"})
	assert.Error(t, err)
}

func TestSecretCRUD(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, err := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "dave", Password: "pass"})
	require.NoError(t, err)
	ctx := authCtx(t, regResp.Token)

	createResp, err := secretClient.CreateSecret(ctx, &pb.CreateSecretRequest{
		Name:    "My Note",
		Type:    pb.SecretType_SECRET_TYPE_TEXT,
		Payload: []byte(`{"content":"hello"}`),
	})
	require.NoError(t, err)
	assert.Equal(t, "My Note", createResp.Meta.Name)

	listResp, err := secretClient.ListSecrets(ctx, &pb.ListSecretsRequest{})
	require.NoError(t, err)
	assert.Len(t, listResp.Secrets, 1)

	getResp, err := secretClient.GetSecret(ctx, &pb.GetSecretRequest{Id: createResp.Meta.Id})
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"content":"hello"}`), getResp.Payload)

	updateResp, err := secretClient.UpdateSecret(ctx, &pb.UpdateSecretRequest{
		Id:      createResp.Meta.Id,
		Name:    "My Note",
		Payload: []byte(`{"content":"updated"}`),
		Version: createResp.Meta.Version,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), updateResp.Meta.Version)

	_, err = secretClient.DeleteSecret(ctx, &pb.DeleteSecretRequest{Id: createResp.Meta.Id})
	require.NoError(t, err)

	listResp2, err := secretClient.ListSecrets(ctx, &pb.ListSecretsRequest{})
	require.NoError(t, err)
	assert.Empty(t, listResp2.Secrets)
}

func TestSecretCRUD_Unauthenticated(t *testing.T) {
	_, _, secretClient := newTestServer(t)

	_, err := secretClient.ListSecrets(context.Background(), &pb.ListSecretsRequest{})
	assert.Error(t, err)
}

func TestUpdateSecret_VersionConflict(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, _ := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "eve", Password: "pass"})
	ctx := authCtx(t, regResp.Token)

	createResp, err := secretClient.CreateSecret(ctx, &pb.CreateSecretRequest{
		Name:    "conflict",
		Type:    pb.SecretType_SECRET_TYPE_TEXT,
		Payload: []byte(`{"content":"v1"}`),
	})
	require.NoError(t, err)

	_, err = secretClient.UpdateSecret(ctx, &pb.UpdateSecretRequest{
		Id:      createResp.Meta.Id,
		Name:    "conflict",
		Payload: []byte(`{"content":"v2"}`),
		Version: 99, // wrong version
	})
	assert.Error(t, err)
}

func TestGetSecret_NotFound(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, _ := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "frank", Password: "pass"})
	ctx := authCtx(t, regResp.Token)

	_, err := secretClient.GetSecret(ctx, &pb.GetSecretRequest{Id: "nonexistent"})
	assert.Error(t, err)
}

func TestDeleteSecret_NotFound(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, _ := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "grace", Password: "pass"})
	ctx := authCtx(t, regResp.Token)

	_, err := secretClient.DeleteSecret(ctx, &pb.DeleteSecretRequest{Id: "no-such"})
	assert.Error(t, err)
}

func TestCreateSecret_EmptyName(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, _ := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "hank", Password: "pass"})
	ctx := authCtx(t, regResp.Token)

	_, err := secretClient.CreateSecret(ctx, &pb.CreateSecretRequest{
		Name:    "",
		Type:    pb.SecretType_SECRET_TYPE_TEXT,
		Payload: []byte("data"),
	})
	assert.Error(t, err)
}

func TestUploadAndDownloadFile(t *testing.T) {
	_, authClient, secretClient := newTestServer(t)

	regResp, err := authClient.Register(context.Background(), &pb.RegisterRequest{Login: "ivan", Password: "pass"})
	require.NoError(t, err)
	ctx := authCtx(t, regResp.Token)

	// Upload.
	uploadStream, err := secretClient.UploadFile(ctx)
	require.NoError(t, err)

	err = uploadStream.Send(&pb.UploadFileRequest{
		Data: &pb.UploadFileRequest_Header{Header: &pb.UploadHeader{Name: "file.bin"}},
	})
	require.NoError(t, err)

	err = uploadStream.Send(&pb.UploadFileRequest{
		Data: &pb.UploadFileRequest_Chunk{Chunk: []byte("hello world")},
	})
	require.NoError(t, err)

	uploadResp, err := uploadStream.CloseAndRecv()
	require.NoError(t, err)
	assert.Equal(t, "file.bin", uploadResp.Meta.Name)

	// Download.
	downloadStream, err := secretClient.DownloadFile(ctx, &pb.DownloadFileRequest{Id: uploadResp.Meta.Id})
	require.NoError(t, err)

	// First message should be meta.
	msg, err := downloadStream.Recv()
	require.NoError(t, err)
	_, ok := msg.Data.(*pb.DownloadFileResponse_Meta)
	assert.True(t, ok)
}
