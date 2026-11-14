package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jur4ikoff/user-service/internal/config"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/rs/zerolog/log"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap admin failed:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "administrator email")
	login := flag.String("login", "", "administrator login")
	firstname := flag.String("firstname", "", "administrator first name")
	lastname := flag.String("lastname", "", "administrator last name")
	passwordStdin := flag.Bool("password-stdin", false, "read password from standard input")
	flag.Parse()
	if !*passwordStdin {
		return errors.New("--password-stdin is required")
	}
	password, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
	if err != nil || len(password) == 0 || len(password) > 4096 {
		return errors.New("read password from standard input")
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r")
	cfg, err := config.LoadBootstrap()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ctx = log.Logger.WithContext(ctx)
	pool, err := infrastructure.NewPool(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns, cfg.DatabaseMinConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	bootstrap := service.NewBootstrapAdminService(repository.NewUserRepository(pool), infrastructure.NewPasswordHasher(cfg.PasswordWorkers), infrastructure.IDGenerator{}, infrastructure.Clock{})
	user, existed, err := bootstrap.Create(ctx, service.RegisterInput{Login: *login, Email: *email, Firstname: *firstname, Lastname: *lastname, Password: value})
	if err != nil {
		return err
	}
	if existed {
		fmt.Printf("administrator %s already exists\n", user.Email)
	} else {
		fmt.Printf("administrator %s created\n", user.Email)
	}
	return nil
}
