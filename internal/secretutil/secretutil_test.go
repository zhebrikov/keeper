package secretutil

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
		{"  TEXT  ", pb.SecretType_SECRET_TYPE_TEXT},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, ParseSecretType(tc.input), "input=%q", tc.input)
	}
}

func TestSecretTypeName(t *testing.T) {
	tests := []struct {
		typ  pb.SecretType
		want string
	}{
		{pb.SecretType_SECRET_TYPE_CREDENTIAL, "credential"},
		{pb.SecretType_SECRET_TYPE_TEXT, "text"},
		{pb.SecretType_SECRET_TYPE_BINARY, "binary"},
		{pb.SecretType_SECRET_TYPE_CARD, "card"},
		{pb.SecretType_SECRET_TYPE_OTP, "otp"},
		{pb.SecretType_SECRET_TYPE_UNSPECIFIED, "unknown"},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, SecretTypeName(tc.typ))
	}
}

func TestFormatSecretData(t *testing.T) {
	data := []byte{0x00, 0xFF}
	require.Equal(t, string(data), FormatSecretData(pb.SecretType_SECRET_TYPE_TEXT, data))
	require.NotEqual(t, string(data), FormatSecretData(pb.SecretType_SECRET_TYPE_BINARY, data))
	require.Equal(t, "AP8=", FormatSecretData(pb.SecretType_SECRET_TYPE_BINARY, data))
}
