package misc

import (
	multierror "github.com/hashicorp/go-multierror"
)

// ErrorOrNil is a variant of multierror.(*Error).ErrorOrNil that always makes
// defensive copies of the error and its list, rather than re-using them.  It
// also auto-flattens the list.
func ErrorOrNil(multi multierror.Error) error {
	length := uint(len(multi.Errors))

	if length == 0 {
		return nil
	}

	if length == 1 {
		_, ok := multi.Errors[0].(*multierror.Error)
		if !ok {
			return multi.Errors[0]
		}
	}

	clone := &multierror.Error{
		Errors:      make([]error, 0, len(multi.Errors)),
		ErrorFormat: multi.ErrorFormat,
	}
	flatten(clone, multi.Errors...)
	return clone
}

func flatten(out *multierror.Error, errs ...error) {
	for _, e := range errs {
		switch x := e.(type) {
		case *multierror.Error:
			flatten(out, x.Errors...)
		default:
			out.Errors = append(out.Errors, e)
		}
	}
}
