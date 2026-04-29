/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package backend

import (
	"errors"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/rules"
)

var (
	ErrOrphanedRule         = errors.New("rule found without directory")
	ErrInvalidDir           = errors.New("invalid dir path")
	ErrInvalidUser          = errors.New("invalid user")
	ErrCannotClaimDirectory = errors.New("cannot claim directory")
	ErrInvalidFrequency     = errors.New("invalid frequency")
	ErrInvalidAction        = errors.New("invalid action")
	ErrInvalidMatch         = errors.New("invalid match string")
	ErrInvalidTime          = errors.New("invalid time")
	ErrDirectoryNotFrozen   = errors.New("directory not frozen")
	ErrAlreadyFrozen        = errors.New("directory already frozen")
	ErrNoIBackup            = errors.New("no ibackup server registered")
	ErrNotFound             = errors.New("404 page not found")
	ErrNotAuthorised        = errors.New("not authorised to see this directory")
	ErrDirectoryClaimed     = rules.ErrDirectoryClaimed
	ErrDirectoryNotClaimed  = rules.ErrDirectoryNotClaimed
	ErrRuleExists           = rules.ErrRuleExists
	ErrNoRule               = rules.ErrNoRule
	ErrNoName               = errors.New("no name provided")
	ErrNameExists           = errors.New("collection with that name already exists")
	ErrCollectionNotFound   = errors.New("collection not found")
	ErrInvalidID            = errors.New("invalid collection ID")
)

var httpErrors = map[error]int{ //nolint:gochecknoglobals
	ErrNotFound:             http.StatusNotFound,
	ErrNotAuthorised:        http.StatusUnauthorized,
	ErrInvalidDir:           http.StatusBadRequest,
	ErrInvalidUser:          http.StatusForbidden,
	ErrDirectoryClaimed:     http.StatusNotAcceptable,
	ErrCannotClaimDirectory: http.StatusNotAcceptable,
	ErrDirectoryNotClaimed:  http.StatusNotAcceptable,
	ErrRuleExists:           http.StatusBadRequest,
	ErrInvalidFrequency:     http.StatusBadRequest,
	ErrInvalidAction:        http.StatusBadRequest,
	ErrInvalidMatch:         http.StatusBadRequest,
	ErrInvalidTime:          http.StatusBadRequest,
	ErrNoRule:               http.StatusBadRequest,
	ErrDirectoryNotFrozen:   http.StatusBadRequest,
	ErrAlreadyFrozen:        http.StatusBadRequest,
	ErrNoIBackup:            http.StatusNotImplemented,
	ErrNoName:               http.StatusBadRequest,
	ErrNameExists:           http.StatusBadRequest,
	ErrCollectionNotFound:   http.StatusBadRequest,
	ErrInvalidID:            http.StatusBadRequest,
}
