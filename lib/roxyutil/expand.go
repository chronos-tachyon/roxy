package roxyutil

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	multierror "github.com/hashicorp/go-multierror"
)

// ExpandString expands ${ENV_VAR} references.
func ExpandString(in string) (string, error) {
	var errs multierror.Error

	expanded := os.Expand(in, func(name string) string {
		value, found := os.LookupEnv(name)
		if !found {
			err := EnvVarLookupError{Var: name, Err: ErrNotExist}
			errs.Errors = append(errs.Errors, err)
		}
		return value
	})

	err := errs.ErrorOrNil()
	return expanded, err
}

// ExpandPath expands ${ENV_VAR} references, ~ and ~user references, and makes
// the path absolute.
func ExpandPath(in string) (string, error) {
	var errs multierror.Error

	expanded, err := ExpandString(in)
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
		if value, found := os.LookupEnv("HOME"); found && userName == "" {
			homeDir = value
		} else {
			var u *user.User
			u, err = LookupUserByName(userName)
			if err == nil {
				homeDir = u.HomeDir
			} else if userName == "" {
				homeDir = "/home/self"
				errs.Errors = append(errs.Errors, err)
			} else {
				homeDir = filepath.Join("/home", userName)
				errs.Errors = append(errs.Errors, err)
			}
		}
		expanded = filepath.Join(homeDir, rest)
	}

	if expanded != "" && expanded[0] != '\x00' && expanded[0] != '@' && !reURLScheme.MatchString(expanded) {
		abs, err := PathAbs(expanded)
		if err != nil {
			errs.Errors = append(errs.Errors, err)
			abs = expanded
		}
		expanded = filepath.Clean(abs)
	}

	err = errs.ErrorOrNil()
	return expanded, err
}

// ExpandPassword expands ${ENV_VAR} references and @file references.
func ExpandPassword(in string) (string, error) {
	var errs multierror.Error

	expanded, err := ExpandString(in)
	if err != nil {
		if merr, ok := err.(*multierror.Error); ok {
			errs.Errors = merr.Errors
		} else {
			errs.Errors = append(errs.Errors, err)
		}
	}

	if expanded != "" && expanded[0] == '@' {
		raw, err := ioutil.ReadFile(expanded[1:])
		if err != nil {
			return "", err
		}
		expanded = strings.Trim(string(raw), " \t\r\n")
	}

	err = errs.ErrorOrNil()
	return expanded, err
}
