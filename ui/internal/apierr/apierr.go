// Package apierr gives a service error a kind, so a transport that wants a
// status code can pick one. A transport with no use for kinds shows the message
package apierr

import (
	"errors"
	"fmt"
)

// Kind classifies a failure by who can fix it
type Kind int

const (
	// Internal is the kind of any error that carries none
	Internal Kind = iota
	BadRequest
	NotFound
	Conflict
	BadGateway
	TooLarge
)

// Error is a failure with a kind. Msg is what the user sees and Err, when set,
// is the cause for errors.Is and errors.As
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

// New returns an error of the given kind with a fixed message
func New(kind Kind, msg string) error { return &Error{Kind: kind, Msg: msg} }

// Newf is New with a format
func Newf(kind Kind, format string, args ...any) error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// Wrap returns err under a kind, with prefix ahead of its message. An empty
// prefix keeps the message as it was
func Wrap(kind Kind, prefix string, err error) error {
	msg := err.Error()
	if prefix != "" {
		msg = prefix + ": " + msg
	}
	return &Error{Kind: kind, Msg: msg, Err: err}
}

// KindOf returns the kind of the first Error in err's chain, or Internal
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return Internal
}
