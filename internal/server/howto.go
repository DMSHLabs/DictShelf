// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/DMSHLabs/DictShelf/internal/howto"
)

// handleHowtoCopy puts an editable copy of the wudict howto into the folder
// imports go to, then rescans: the copy is listed in place of the built-in
// guide (Builtin), and the user can open it in any editor and watch a change
// appear. The same destination and the same rules as an import (intake.go).
//
// With restore=1 it brings the built-in guide back after the user removed it
// (howto.MarkRemoved): installed again, listed again, and kept from then on.
func (s *Server) handleHowtoCopy(w http.ResponseWriter, r *http.Request) {
	if restore, _ := queryFlag(r.URL.Query(), "restore"); restore {
		value, err := s.howtoOperation(true)
		s.writeHowtoResult(w, value, err)
		return
	}
	value, err := s.howtoOperation(false)
	s.writeHowtoResult(w, value, err)
}

type howtoOperationError struct {
	status  int
	message string
}

func (e *howtoOperationError) Error() string { return e.message }

func (s *Server) howtoOperation(restore bool) (map[string]string, error) {
	if !restore {
		dir := s.importDir()
		if dir == "" {
			return nil, &howtoOperationError{status: http.StatusConflict, message: "no dictionary folder is configured"}
		}
		p, err := howto.CopyTo(dir)
		if errors.Is(err, fs.ErrExist) {
			return nil, &howtoOperationError{status: http.StatusConflict, message: fmt.Sprintf("%s is already in %s", howto.FileName, dir)}
		}
		if err != nil {
			return nil, &howtoOperationError{status: http.StatusInternalServerError, message: err.Error()}
		}
		if err := s.reg.Rescan(); err != nil {
			return nil, &howtoOperationError{status: http.StatusInternalServerError, message: err.Error()}
		}
		s.reg.Warm()
		return map[string]string{"path": p}, nil
	}
	if s.User.Builtin() == "" {
		return nil, &howtoOperationError{status: http.StatusConflict, message: "this wudict has no place to keep the wudict howto"}
	}
	p, err := howto.Restore(s.User.Builtin())
	if err != nil {
		return nil, &howtoOperationError{status: http.StatusInternalServerError, message: err.Error()}
	}
	s.reg.AddBuiltin(Builtin{ID: howto.ID, Path: p})
	if err := s.reg.Rescan(); err != nil {
		return nil, &howtoOperationError{status: http.StatusInternalServerError, message: err.Error()}
	}
	s.reg.Warm()
	return map[string]string{"path": p}, nil
}

func (s *Server) writeHowtoResult(w http.ResponseWriter, value map[string]string, err error) {
	if err != nil {
		var failure *howtoOperationError
		if errors.As(err, &failure) {
			httpErr(w, failure.status, "%s", failure.message)
			return
		}
		httpErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, value)
}
