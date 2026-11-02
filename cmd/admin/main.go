// Command admin provisions accounts and API keys. It replaces public signup:
// the operator creates every login and every machine credential.
//
//	admin create-user    -email ops@example.com -role admin   (password on stdin)
//	admin create-api-key -email ops@example.com -name data-to-video \
//	                     -scopes posts:create,posts:read [-ttl 2160h]
//
// It reads the same DB_* environment as the other binaries.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "create-user":
		err = createUser(ctx, os.Args[2:])
	case "create-api-key":
		err = createAPIKey(ctx, os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: admin create-user -email E [-name N] [-role admin|creator|editor|super_admin] < password")
	fmt.Fprintln(os.Stderr, "       admin create-api-key -email E -name N -scopes posts:create,posts:read [-ttl 2160h]")
	os.Exit(2)
}

func createUser(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	email := fs.String("email", "", "login email")
	name := fs.String("name", "", "display name (defaults to email)")
	role := fs.String("role", "admin", "role: admin, creator, editor or super_admin")
	_ = fs.Parse(args)

	// Password comes from stdin so it never appears in shell history or ps.
	fmt.Fprintf(os.Stderr, "password (min %d chars): ", service.MinPasswordLength)
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")

	pool, err := database.Connect(config.Load().DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	id, err := service.CreateUser(ctx, pool, *email, *name, *role, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\ncreated user %s (%s)\n", id, *role)
	return nil
}

func createAPIKey(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("create-api-key", flag.ExitOnError)
	email := fs.String("email", "", "owner account; posts created with the key are authored by it")
	name := fs.String("name", "", "label for the key, e.g. data-to-video")
	scopes := fs.String("scopes", "posts:create,posts:read", "comma-separated scopes")
	ttl := fs.Duration("ttl", 90*24*time.Hour, "lifetime; 0 means no expiry")
	_ = fs.Parse(args)

	pool, err := database.Connect(config.Load().DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	raw, err := service.IssueAPIKey(ctx, pool, *email, *name, strings.Split(*scopes, ","), *ttl)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "API key (shown once; only its SHA-256 is stored):")
	fmt.Println(raw)
	return nil
}
