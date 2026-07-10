package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zhebrikov/gophkeeper/internal/config"
	"github.com/zhebrikov/gophkeeper/internal/crypto"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

func newRegisterCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	var login, password string

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new account",
		Run: func(cmd *cobra.Command, args []string) {
			if login == "" {
				var err error
				login, err = readLine("Login: ")
				exitOnError(err)
			}
			if password == "" {
				var err error
				password, err = readPassword("Password: ")
				exitOnError(err)
			}

			api, err := newGophkeeperClient(cfg)
			exitOnError(err)
			defer api.Close()

			exitOnError(api.Register(ctx, login, password))
			fmt.Println("Registration successful")
		},
	}

	cmd.Flags().StringVar(&login, "login", "", "account login")
	cmd.Flags().StringVar(&password, "password", "", "account password")
	return cmd
}

func newLoginCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	var login, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the server",
		Run: func(cmd *cobra.Command, args []string) {
			if login == "" {
				var err error
				login, err = readLine("Login: ")
				exitOnError(err)
			}
			if password == "" {
				var err error
				password, err = readPassword("Password: ")
				exitOnError(err)
			}

			api, err := newGophkeeperClient(cfg)
			exitOnError(err)
			defer api.Close()

			exitOnError(api.Login(ctx, login, password))
			fmt.Println("Login successful")
		},
	}

	cmd.Flags().StringVar(&login, "login", "", "account login")
	cmd.Flags().StringVar(&password, "password", "", "account password")
	return cmd
}

func newLogoutCmd(cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear local session",
		Run: func(cmd *cobra.Command, args []string) {
			api, err := newGophkeeperClient(cfg)
			exitOnError(err)
			defer api.Close()

			exitOnError(api.Logout())
			fmt.Println("Logged out")
		},
	}
}

func newSyncCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Synchronize secrets with the server",
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			resp, err := api.Sync(ctx)
			exitOnError(err)
			fmt.Printf("Synced %d secret(s)\n", len(resp.GetSecrets()))
		},
	}
}

func newListCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored secrets",
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			resp, err := api.ListSecrets(ctx, time.Time{})
			exitOnError(err)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTYPE\tNAME\tVERSION\tUPDATED")
			for _, s := range resp.GetSecrets() {
				if s.GetDeleted() {
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
					s.GetId(), secretTypeName(s.GetType()), s.GetName(), s.GetVersion(),
					formatUnix(s.GetUpdatedAt()),
				)
			}
			_ = w.Flush()
		},
	}
}

func newGetCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get and decrypt a secret by ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			master, err := readMasterPassword()
			exitOnError(err)

			secret, err := api.GetSecret(ctx, args[0])
			exitOnError(err)

			plaintext, err := crypto.DecryptWithPassword(master, secret.GetEncryptedData())
			exitOnError(err)

			fmt.Printf("ID:       %s\n", secret.GetId())
			fmt.Printf("Type:     %s\n", secretTypeName(secret.GetType()))
			fmt.Printf("Name:     %s\n", secret.GetName())
			fmt.Printf("Metadata: %s\n", secret.GetMetadata())
			fmt.Printf("Version:  %d\n", secret.GetVersion())
			fmt.Printf("Data:     %s\n", formatSecretData(secret.GetType(), plaintext))
		},
	}
}

func newAddCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	var secretType, name, metadata, data string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new secret",
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			if secretType == "" {
				secretType, err = readLine("Type (credential/text/binary/card/otp): ")
				exitOnError(err)
			}
			if name == "" {
				name, err = readLine("Name: ")
				exitOnError(err)
			}
			if data == "" {
				data, err = readLine("Data: ")
				exitOnError(err)
			}
			if metadata == "" {
				metadata, _ = readLine("Metadata (optional): ")
			}

			master, err := readMasterPassword()
			exitOnError(err)

			encrypted, err := crypto.EncryptWithPassword(master, []byte(data))
			exitOnError(err)

			_, err = api.CreateSecret(ctx, &pb.CreateSecretRequest{
				Type:          parseSecretType(secretType),
				Name:          name,
				EncryptedData: encrypted,
				Metadata:      metadata,
			})
			exitOnError(err)
			fmt.Println("Secret created")
		},
	}

	cmd.Flags().StringVar(&secretType, "type", "", "secret type")
	cmd.Flags().StringVar(&name, "name", "", "secret name")
	cmd.Flags().StringVar(&metadata, "metadata", "", "optional metadata")
	cmd.Flags().StringVar(&data, "data", "", "secret data")
	return cmd
}

func newUpdateCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	var name, metadata, data string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an existing secret",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			existing, err := api.GetSecret(ctx, args[0])
			exitOnError(err)

			if name == "" {
				name = existing.GetName()
			}
			if data == "" {
				data, err = readLine("New data: ")
				exitOnError(err)
			}
			if metadata == "" {
				metadata = existing.GetMetadata()
			}

			master, err := readMasterPassword()
			exitOnError(err)

			encrypted, err := crypto.EncryptWithPassword(master, []byte(data))
			exitOnError(err)

			_, err = api.UpdateSecret(ctx, &pb.UpdateSecretRequest{
				Id:            args[0],
				Name:          name,
				EncryptedData: encrypted,
				Metadata:      metadata,
				Version:       existing.GetVersion(),
			})
			exitOnError(err)
			fmt.Println("Secret updated")
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&metadata, "metadata", "", "new metadata")
	cmd.Flags().StringVar(&data, "data", "", "new data")
	return cmd
}

func newDeleteCmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			api, err := requireAuth(cfg)
			exitOnError(err)
			defer api.Close()

			exitOnError(api.DeleteSecret(ctx, args[0]))
			fmt.Println("Secret deleted")
		},
	}
}

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
	return fmt.Sprintf("%d", ts)
}
