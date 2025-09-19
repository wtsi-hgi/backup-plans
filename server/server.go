package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/httpbuffer"

	_ "vimagination.zapto.org/httpbuffer/gzip"
)

type Server struct {
	getUser func(r *http.Request) string

	rulesMu      sync.RWMutex
	rulesDB      *db.DB
	rules        map[string]*DirRules
	stateMachine group.StateMachine[db.Rule]

	treeMu    sync.RWMutex
	maps      map[string]func()
	structure TopLevelDir
}

func New(d *db.DB, getUser func(r *http.Request) string) (*Server, error) {
	s := &Server{
		getUser: getUser,
		rulesDB: d,
		maps:    make(map[string]func()),
		structure: TopLevelDir{
			children: map[string]Node{},
		},
	}

	if err := s.loadRules(); err != nil {
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
