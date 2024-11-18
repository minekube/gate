package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"

	"go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1/gatev1connect"
)

func NewServer(cfg Config, h Handler) *Server {
	return &Server{
		cfg: cfg,
		h:   h,
	}
}

type Server struct {
	cfg Config
	h   Handler
}

func (s *Server) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("starting api service", "bind", s.cfg.Bind)

	mux := http.NewServeMux()
	mux.Handle(gatev1connect.NewGateServiceHandler(s.h))

	hs := &http.Server{
		Addr: s.cfg.Bind,
		Handler: h2c.NewHandler(mux, &http2.Server{
			IdleTimeout: time.Second * 30,
		}),
		ReadTimeout:       time.Second * 5,
		ReadHeaderTimeout: time.Second * 5,
		WriteTimeout:      time.Second * 10,
		IdleTimeout:       time.Second * 30,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		<-ctx.Done()
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		return hs.Shutdown(stopCtx)
	})
	eg.Go(func() error { return ignoreClosed(hs.ListenAndServe()) })

	return eg.Wait()
}

func ignoreClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
