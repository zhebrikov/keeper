package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	gkclient "github.com/zhebrikov/gophkeeper/internal/client"
	"github.com/zhebrikov/gophkeeper/internal/crypto"
)

type secretsLoadedMsg struct {
	secrets []*pb.SecretResponse
	err     error
}

type secretDetailMsg struct {
	secret    *pb.SecretResponse
	plaintext []byte
	err       error
}

type actionDoneMsg struct {
	message string
	err     error
}

type authDoneMsg struct {
	register bool
	err      error
}

func loadSecretsCmd(ctx context.Context, api *gkclient.API) tea.Cmd {
	return func() tea.Msg {
		resp, err := api.ListSecrets(ctx, time.Time{})
		if err != nil {
			return secretsLoadedMsg{err: err}
		}
		var secrets []*pb.SecretResponse
		for _, s := range resp.GetSecrets() {
			if s.GetDeleted() {
				continue
			}
			secrets = append(secrets, s)
		}
		return secretsLoadedMsg{secrets: secrets}
	}
}

func getSecretCmd(ctx context.Context, api *gkclient.API, id, master string) tea.Cmd {
	return func() tea.Msg {
		secret, err := api.GetSecret(ctx, id)
		if err != nil {
			return secretDetailMsg{err: err}
		}
		plaintext, err := crypto.DecryptWithPassword(master, secret.GetEncryptedData())
		if err != nil {
			return secretDetailMsg{err: err}
		}
		return secretDetailMsg{secret: secret, plaintext: plaintext}
	}
}

func authCmd(ctx context.Context, api *gkclient.API, register bool, login, password string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if register {
			err = api.Register(ctx, login, password)
		} else {
			err = api.Login(ctx, login, password)
		}
		return authDoneMsg{register: register, err: err}
	}
}

func createSecretCmd(ctx context.Context, api *gkclient.API, master string, typ pb.SecretType, name, data, metadata string) tea.Cmd {
	return func() tea.Msg {
		encrypted, err := crypto.EncryptWithPassword(master, []byte(data))
		if err != nil {
			return actionDoneMsg{err: err}
		}
		_, err = api.CreateSecret(ctx, &pb.CreateSecretRequest{
			Type:          typ,
			Name:          name,
			EncryptedData: encrypted,
			Metadata:      metadata,
		})
		return actionDoneMsg{message: "Secret created", err: err}
	}
}

func updateSecretCmd(ctx context.Context, api *gkclient.API, master, id, name, data, metadata string, version int64) tea.Cmd {
	return func() tea.Msg {
		encrypted, err := crypto.EncryptWithPassword(master, []byte(data))
		if err != nil {
			return actionDoneMsg{err: err}
		}
		_, err = api.UpdateSecret(ctx, &pb.UpdateSecretRequest{
			Id:            id,
			Name:          name,
			EncryptedData: encrypted,
			Metadata:      metadata,
			Version:       version,
		})
		return actionDoneMsg{message: "Secret updated", err: err}
	}
}

func deleteSecretCmd(ctx context.Context, api *gkclient.API, id string) tea.Cmd {
	return func() tea.Msg {
		err := api.DeleteSecret(ctx, id)
		return actionDoneMsg{message: "Secret deleted", err: err}
	}
}

func syncCmd(ctx context.Context, api *gkclient.API) tea.Cmd {
	return func() tea.Msg {
		resp, err := api.Sync(ctx)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{message: fmt.Sprintf("Synced %d secret(s)", len(resp.GetSecrets()))}
	}
}

func logoutCmd(api *gkclient.API) tea.Cmd {
	return func() tea.Msg {
		err := api.Logout()
		return actionDoneMsg{message: "Logged out", err: err}
	}
}
