package vieneu

import "errors"

var (
	ErrDaemonUnreachable = errors.New("vieneu: daemon unreachable")
	ErrRefAudioInvalid   = errors.New("vieneu: reference audio invalid")
	ErrSynthFailed       = errors.New("vieneu: synthesis failed")
	ErrVoicesFetchFailed = errors.New("vieneu: voices fetch failed")
)
