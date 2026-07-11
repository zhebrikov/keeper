package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

func TestSecretsToItems(t *testing.T) {
	secrets := []*pb.SecretResponse{
		{Id: "1", Name: "github", Type: pb.SecretType_SECRET_TYPE_CREDENTIAL},
	}
	items := secretsToItems(secrets)
	require.Len(t, items, 1)
	require.Equal(t, "github", items[0].(secretItem).Title())
}

func TestOpenFormEdit(t *testing.T) {
	m := model{
		formInputs: newFormInputs(),
	}
	secret := &pb.SecretResponse{
		Id:       "id-1",
		Type:     pb.SecretType_SECRET_TYPE_TEXT,
		Name:     "note",
		Metadata: "meta",
		Version:  2,
	}
	m.openForm(formEdit, secret, "plain")
	require.Equal(t, screenForm, m.screen)
	require.Equal(t, formEdit, m.formMode)
	require.Equal(t, "note", m.formInputs[1].Value())
	require.Equal(t, "plain", m.formInputs[2].Value())
	require.Equal(t, int64(2), m.editingVersion)
}
