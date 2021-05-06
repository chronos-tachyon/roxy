package mainutil

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	multierror "github.com/hashicorp/go-multierror"
)

func ProcessString(in string) (string, error) {
	var errs multierror.Error

	expanded := os.Expand(in, func(name string) string {
		value, found := os.LookupEnv(name)
		if !found {
			err := fmt.Errorf("unknown environment variable \"$%s\"", name)
			errs.Errors = append(errs.Errors, err)
		}
		return value
	})

	if err := errs.ErrorOrNil(); err != nil {
		return "", err
	}

	return expanded, nil
}

func ProcessPath(in string) (string, error) {
	var errs multierror.Error

	expanded, err := ProcessString(in)
	if err != nil {
		if merr, ok := err.(*multierror.Error); ok {
			errs.Errors = merr.Errors
		} else {
			errs.Errors = append(errs.Errors, err)
		}
	}

	if expanded != "" && expanded[0] == '~' {
		var userName string
		var rest string
		if i := strings.IndexByte(expanded, '/'); i >= 0 {
			userName, rest = expanded[1:i], expanded[i+1:]
		} else {
			userName = expanded[1:]
		}

		var homeDir string
		if userName == "" {
			if value, found := os.LookupEnv("HOME"); found {
				homeDir = value
			} else if u, err := user.Current(); err == nil {
				homeDir = u.HomeDir
			} else {
				err = fmt.Errorf("failed to look up current user: %w", err)
				errs.Errors = append(errs.Errors, err)
				homeDir = "/home/self"
			}
		} else {
			if u, err := user.Lookup(userName); err == nil {
				homeDir = u.HomeDir
			} else {
				err = fmt.Errorf("failed to look up user %q: %w", userName, err)
				errs.Errors = append(errs.Errors, err)
				homeDir = "/home/" + userName
			}
		}

		expanded = filepath.Join(homeDir, rest)
	}

	if expanded != "" && expanded[0] != '@' && !regexp.MustCompile(`^[0-9A-Za-z+-]+:`).MatchString(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			err = fmt.Errorf("failed to make path %q absolute: %w", expanded, err)
			errs.Errors = append(errs.Errors, err)
		}
		expanded = filepath.Clean(abs)
	}

	if err = errs.ErrorOrNil(); err != nil {
		return "", err
	}

	return expanded, nil
}
