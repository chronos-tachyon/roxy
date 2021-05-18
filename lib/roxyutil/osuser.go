package roxyutil

import (
	"os/user"
	"strconv"
)

// LookupUserByName is a wrapper around "os/user".Lookup.
func LookupUserByName(userName string) (*user.User, error) {
	var u *user.User
	var err error
	if userName == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(userName)
	}
	err = flattenUserLookupError(err)
	if err != nil {
		return nil, LookupUserByNameError{Name: userName, Err: err}
	}
	return u, nil
}

// LookupUserByID is a wrapper around "os/user".LookupId.
func LookupUserByID(uid uint32) (*user.User, error) {
	u, err := user.LookupId(uint32ToString(uid))
	err = flattenUserLookupError(err)
	if err != nil {
		return nil, LookupUserByIDError{ID: uid, Err: err}
	}
	return u, nil
}

// LookupGroupByName is a wrapper around "os/user".LookupGroup.
func LookupGroupByName(groupName string) (*user.Group, error) {
	g, err := user.LookupGroup(groupName)
	err = flattenUserLookupError(err)
	if err != nil {
		return nil, LookupGroupByNameError{Name: groupName, Err: err}
	}
	return g, nil
}

// LookupGroupByID is a wrapper around "os/user".LookupGroupId.
func LookupGroupByID(gid uint32) (*user.Group, error) {
	g, err := user.LookupGroupId(uint32ToString(gid))
	err = flattenUserLookupError(err)
	if err != nil {
		return nil, LookupGroupByIDError{ID: gid, Err: err}
	}
	return g, nil
}

func uint32ToString(x uint32) string {
	return strconv.FormatUint(uint64(x), 10)
}

func flattenUserLookupError(err error) error {
	switch err.(type) {
	case user.UnknownUserError:
		return ErrNotExist
	case user.UnknownUserIdError:
		return ErrNotExist
	case user.UnknownGroupError:
		return ErrNotExist
	case user.UnknownGroupIdError:
		return ErrNotExist
	default:
		return err
	}
}
