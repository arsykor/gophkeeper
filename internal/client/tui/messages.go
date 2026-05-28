// Package tui implements the terminal user interface using bubbletea.
package tui

import (
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"
)

// Screen identifies which model is currently active.
type Screen int

const (
	ScreenLogin Screen = iota
	ScreenList
	ScreenDetail
	ScreenCreate
	ScreenEdit
)

// msgAuthDone is sent when login/register succeeds.
type msgAuthDone struct {
	token string
}

// msgSecretsLoaded is sent when the secret list is fetched.
type msgSecretsLoaded struct {
	secrets []*pb.SecretMeta
	err     error
}

// msgSecretDetail is sent when a secret's payload is fetched.
type msgSecretDetail struct {
	resp *pb.GetSecretResponse
	err  error
}

// msgSaved is sent after create or update completes.
type msgSaved struct {
	meta *pb.SecretMeta
	err  error
}

// msgDeleted is sent after a delete completes.
type msgDeleted struct{ err error }

// msgError carries a generic error to display.
type msgError struct{ err error }
