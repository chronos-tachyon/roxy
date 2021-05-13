package roxyutil

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	multierror "github.com/hashicorp/go-multierror"
)

// ExpandString expands ${ENV_VAR} references.
func ExpandString(in string) (string, error) {
	var errs multierror.Error

	expanded := os.Expand(in, func(name string) string {
		value, found := os.LookupEnv(name)
		if !found {
			err := BadEnvVarError{Var: name}
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

		var u *user.User
		if userName == "" {
			if value, found := os.LookupEnv("HOME"); found {
				u = &user.User{HomeDir: value}
				err = nil
			} else {
				u, err = user.Current()
				if err != nil {
					u = &user.User{HomeDir: "/home/self"}
				}
			}
		} else {
			u, err = user.Lookup(userName)
			if err != nil {
				u = &user.User{HomeDir: filepath.Join("/home", userName)}
			}
		}
		if err != nil {
			err = FailedUserNameLookupError{Name: userName, Err: err}
			errs.Errors = append(errs.Errors, err)
		}
		homeDir := u.HomeDir
		expanded = filepath.Join(homeDir, rest)
	}

	if expanded != "" && expanded[0] != '\x00' && expanded[0] != '@' && !regexp.MustCompile(`^[0-9A-Za-z+-]+:`).MatchString(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			err = FailedPathAbsError{Path: expanded, Err: err}
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
