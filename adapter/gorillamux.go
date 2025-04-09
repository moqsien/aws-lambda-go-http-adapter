package adapter

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
)

type gorillaMuxAdapter struct {
	router *mux.Router
}

func (g gorillaMuxAdapter) adapterFunc(ctx context.Context, r *http.Request, w http.ResponseWriter) error {
	g.router.ServeHTTP(w, r)
	return nil
}

func NewGorillaMuxAdapter(delegate *mux.Router) handler.AdapterFunc {
	return gorillaMuxAdapter{router: delegate}.adapterFunc
}
