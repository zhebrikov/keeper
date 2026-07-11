package secretutil

import (
	"encoding/base64"
	"strings"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

func ParseSecretType(raw string) pb.SecretType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "credential", "login":
		return pb.SecretType_SECRET_TYPE_CREDENTIAL
	case "text":
		return pb.SecretType_SECRET_TYPE_TEXT
	case "binary":
		return pb.SecretType_SECRET_TYPE_BINARY
	case "card":
		return pb.SecretType_SECRET_TYPE_CARD
	case "otp":
		return pb.SecretType_SECRET_TYPE_OTP
	default:
		return pb.SecretType_SECRET_TYPE_TEXT
	}
}

func SecretTypeName(t pb.SecretType) string {
	switch t {
	case pb.SecretType_SECRET_TYPE_CREDENTIAL:
		return "credential"
	case pb.SecretType_SECRET_TYPE_TEXT:
		return "text"
	case pb.SecretType_SECRET_TYPE_BINARY:
		return "binary"
	case pb.SecretType_SECRET_TYPE_CARD:
		return "card"
	case pb.SecretType_SECRET_TYPE_OTP:
		return "otp"
	default:
		return "unknown"
	}
}

func FormatSecretData(t pb.SecretType, data []byte) string {
	if t == pb.SecretType_SECRET_TYPE_BINARY {
		return base64.StdEncoding.EncodeToString(data)
	}
	return string(data)
}
