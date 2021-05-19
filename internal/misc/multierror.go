package misc

import (
	multierror "github.com/hashicorp/go-multierror"
)

func ErrorOrNil(multi multierror.Error) error {
	switch uint(len(multi.Errors)) {
	case 0:
		return nil

	case 1:
		return multi.Errors[0]

	default:
		clone := &multierror.Error{
			Errors:      make([]error, 0, len(multi.Errors)),
			ErrorFormat: multi.ErrorFormat,
		}
		flatten(clone, multi.Errors...)
		return clone
	}
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
