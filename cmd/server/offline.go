package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nagayon-935/conduit/internal/control"
	"golang.org/x/term"
)

// Passwords are read from the terminal (without echo) or stdin, never argv/env.
// Run only while the Web server is stopped: Open invalidates old Web logins.
func offline(args []string) error {
	cmd := args[0]
	if cmd != "bootstrap-admin" && cmd != "reset-password" {
		return errors.New("usage: conduit {bootstrap-admin|reset-password} --login NAME [--db PATH]")
	}
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	login := f.String("login", "", "Conduit login name")
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/conduit.db"
	}
	db := f.String("db", dbPath, "database path")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if *login == "" {
		return errors.New("--login is required")
	}
	var password string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "New temporary password (12+ characters): ")
		p, e := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if e != nil {
			return e
		}
		password = string(p)
		clear(p)
	} else {
		r := bufio.NewReaderSize(os.Stdin, 2048)
		p, e := r.ReadString('\n')
		if e != nil && len(p) == 0 {
			return e
		}
		password = strings.TrimRight(p, "\r\n")
	}
	s, e := control.Open(*db)
	if e != nil {
		return e
	}
	defer s.Close()
	if cmd == "bootstrap-admin" {
		if e = s.Bootstrap(*login, password); e != nil {
			return e
		}
	} else {
		users, e := s.Users()
		if e != nil {
			return e
		}
		found := false
		for _, u := range users {
			if u.Login == control.Normalize(*login) {
				hash, e := control.HashPassword(password)
				if e != nil {
					return e
				}
				u.Hash = hash
				u.MustChange = true
				if e = s.SaveUser("offline", u); e != nil {
					return e
				}
				found = true
				break
			}
		}
		if !found {
			return errors.New("login not found")
		}
	}
	fmt.Fprintln(os.Stderr, "Account updated. Password change is required on next login.")
	return nil
}
