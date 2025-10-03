package server

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

func New(d *db.DB, getUser func(r *http.Request) string, reportRoots []string) (*Server, error) {
	s := &Server{
		getUser:     getUser,
		rulesDB:     d,
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

type Error struct {
	Code int
	Err  error
}

func (e Error) Error() string {
	return e.Err.Error()
}
