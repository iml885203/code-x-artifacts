package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iml885203/code-x-artifacts/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

// initAccounts creates a first account without placing a password in arguments
// or logs. Exclusive file creation preserves every existing credential file.
func initAccounts(dir string, out io.Writer) error {
	if err := os.MkdirAll(dir, 0o700); err != nil { //nolint:gosec // G703: the CLI operator explicitly selects the credential directory.
		return fmt.Errorf("create account directory: %w", err)
	}
	var secret [24]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(secret[:])
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	data, err := json.MarshalIndent([]auth.LocalAccount{{Username: "admin", DisplayName: "Administrator", PasswordHash: string(hash), Groups: []string{"members"}}}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode account: %w", err)
	}
	passwordPath := filepath.Join(dir, "admin-password")
	if err := createPrivateFile(passwordPath, []byte(password+"\n")); err != nil {
		return err
	}
	if err := createPrivateFile(filepath.Join(dir, "users.json"), append(data, '\n')); err != nil {
		_ = os.Remove(passwordPath) //nolint:gosec // G703: only the exclusive file this initializer just created.
		return err
	}
	_, err = fmt.Fprintf(out, "Created admin account. Read the password from %s, then store it in your password manager and delete that file.\n", passwordPath)
	return err
}

func createPrivateFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: operator-selected credential path; exclusive creation never follows an existing symlink.
	if err != nil {
		return fmt.Errorf("create private file %s (existing files are never overwritten): %w", path, err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write private file: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close private file: %w", closeErr)
	}
	return nil
}

func runCommand(args []string, in io.Reader, out io.Writer) error {
	usage := func() error {
		return fmt.Errorf("usage: codexartifacts [init [account-directory] | hash-password | healthcheck]")
	}
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "healthcheck":
		if len(args) != 1 {
			return usage()
		}
		return healthcheck(listenAddress())
	case "init":
		switch len(args) {
		case 1:
			return initAccounts(".local", out)
		case 2:
			return initAccounts(args[1], out)
		default:
			return usage()
		}
	case "hash-password":
		if len(args) != 1 {
			return usage()
		}
		password, err := io.ReadAll(io.LimitReader(in, 74))
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		password = []byte(strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r"))
		if len(password) < 12 || len(password) > 72 {
			return fmt.Errorf("password must be 12–72 bytes")
		}
		hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		_, err = fmt.Fprintln(out, string(hash))
		return err
	default:
		return usage()
	}
}
