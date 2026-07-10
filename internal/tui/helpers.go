package tui

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

func parseSecretType(raw string) pb.SecretType {
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

func secretTypeName(t pb.SecretType) string {
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

func formatSecretData(t pb.SecretType, data []byte) string {
	if t == pb.SecretType_SECRET_TYPE_BINARY {
		return base64.StdEncoding.EncodeToString(data)
	}
	return string(data)
}

func formatUnix(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func detailLine(key, value string) string {
	return detailKeyStyle.Render(key+":") + " " + detailValueStyle.Render(value)
}

func footerHelp(keys ...string) string {
	return helpStyle.Render(strings.Join(keys, "  •  "))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func secretListTitle() string {
	return fmt.Sprintf("%s %s", titleStyle.Render("GophKeeper"), subtitleStyle.Render("secrets"))
}
