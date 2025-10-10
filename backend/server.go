package backend

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/httpbuffer"

	_ "vimagination.zapto.org/httpbuffer/gzip"
)

// Server represents all of the data requireed to run the backend server.
type Server struct {
	getUser func(r *http.Request) string

	rulesMu        sync.RWMutex
	rulesDB        *db.DB
	directoryRules map[string]*ruletree.DirRules
	dirs           map[uint64]*db.Directory
	rules          map[uint64]*db.Rule
	stateMachine   group.StateMachine[db.Rule]
	reportRoots    []string

	rootDir *ruletree.RootDir
}

// New creates a new Backend API server.
func New(db *db.DB, getUser func(r *http.Request) string, reportRoots []string) (*Server, error) {
	s := &Server{
		getUser:     getUser,
		rulesDB:     db,
		reportRoots: reportRoots,
	}

	rules, err := s.loadRules()
	if err != nil {
		return nil, err
	}

	s.rootDir, err = ruletree.NewRoot(rules)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// WhoAmI is an HTTP endpoint that returns the result of the getUser func that
// was passed to the New function.
func (s *Server) WhoAmI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(s.getUser(r))
}

func handle(w http.ResponseWriter, r *http.Request, fn func(http.ResponseWriter, *http.Request) error) {
	httpbuffer.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil {
				var errc Error

				if errors.As(err, &errc) {
					http.Error(w, errc.Err.Error(), errc.Code)

					return
				}

				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}),
	}.ServeHTTP(w, r)
}

// Error is an error that contains an HTTP error code.
type Error struct {
	Code int
	Err  error
}

func (e Error) Error() string {
	return e.Err.Error()
}
