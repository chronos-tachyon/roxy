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
	var errors []error

	expanded := os.Expand(in, func(name string) string {
		value, found := os.LookupEnv(name)
		if !found {
			err := EnvVarLookupError{Var: name, Err: ErrNotExist}
			errors = append(errors, err)
		}
		return value
	})

	var err error
	switch uint(len(errors)) {
	case 0:
		err = nil
	case 1:
		err = errors[0]
	default:
		err = &multierror.Error{Errors: errors}
	}
	return expanded, err
}

// ExpandPath expands ${ENV_VAR} references, ~ and ~user references, and makes
// the path absolute (by assuming it is relative to the current directory).
func ExpandPath(in string) (string, error) {
	return ExpandPathWithCWD(in, ".")
}

// ExpandPathWithCWD expands ${ENV_VAR} references, ~ and ~user references, and
// makes the path absolute (by assuming it is relative to the given cwd).
func ExpandPathWithCWD(in string, cwd string) (string, error) {
	var errors []error

	expanded, err := ExpandString(in)
	if err != nil {
		if multi, ok := err.(*multierror.Error); ok {
			errors = multi.Errors
		} else {
			errors = append(errors, err)
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
				errors = append(errors, err)
			} else {
				homeDir = filepath.Join("/home", userName)
				errors = append(errors, err)
			}
		}
		expanded = filepath.Join(homeDir, rest)
	}

	if expanded != "" && expanded[0] != '\x00' && expanded[0] != '@' && !reURLScheme.MatchString(expanded) {
		if !filepath.IsAbs(expanded) {
			expanded = filepath.Join(cwd, expanded)
		}
		if !filepath.IsAbs(expanded) {
			abs, err := PathAbs(expanded)
			if err != nil {
				errors = append(errors, err)
				abs = expanded
			}
			expanded = abs
		}
		expanded = filepath.Clean(expanded)
	}

	switch uint(len(errors)) {
	case 0:
		err = nil
	case 1:
		err = errors[0]
	default:
		err = &multierror.Error{Errors: errors}
	}
	return expanded, err
}

// ExpandPassword expands ${ENV_VAR} references and @file references.
func ExpandPassword(in string) (string, error) {
	var errors []error

	expanded, err := ExpandString(in)
	if err != nil {
		if multi, ok := err.(*multierror.Error); ok {
			errors = multi.Errors
		} else {
			errors = append(errors, err)
		}
	}

	if expanded != "" && expanded[0] == '@' {
		raw, err := ioutil.ReadFile(expanded[1:])
		if err != nil {
			return "", err
		}
		expanded = strings.Trim(string(raw), " \t\r\n")
	}

	switch uint(len(errors)) {
	case 0:
		err = nil
	case 1:
		err = errors[0]
	default:
		err = &multierror.Error{Errors: errors}
	}
	return expanded, err
}
