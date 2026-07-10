package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

func TestParseSecretType(t *testing.T) {
	tests := []struct {
		input string
		want  pb.SecretType
	}{
		{"credential", pb.SecretType_SECRET_TYPE_CREDENTIAL},
		{"login", pb.SecretType_SECRET_TYPE_CREDENTIAL},
		{"text", pb.SecretType_SECRET_TYPE_TEXT},
		{"binary", pb.SecretType_SECRET_TYPE_BINARY},
		{"card", pb.SecretType_SECRET_TYPE_CARD},
		{"otp", pb.SecretType_SECRET_TYPE_OTP},
		{"unknown", pb.SecretType_SECRET_TYPE_TEXT},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, parseSecretType(tc.input), "input=%q", tc.input)
	}
}

func TestSecretTypeName(t *testing.T) {
	require.Equal(t, "credential", secretTypeName(pb.SecretType_SECRET_TYPE_CREDENTIAL))
	require.Equal(t, "unknown", secretTypeName(pb.SecretType_SECRET_TYPE_UNSPECIFIED))
}

func TestFormatSecretData(t *testing.T) {
	data := []byte{0x00, 0xFF}
	require.Equal(t, string(data), formatSecretData(pb.SecretType_SECRET_TYPE_TEXT, data))
	require.Equal(t, "AP8=", formatSecretData(pb.SecretType_SECRET_TYPE_BINARY, data))
}

func TestTruncate(t *testing.T) {
	require.Equal(t, "hello", truncate("hello", 10))
	require.Equal(t, "hel...", truncate("hello world", 6))
}

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
