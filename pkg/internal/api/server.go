package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"

	"go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1/gatev1connect"
)

func NewServer(cfg Config, h gatev1connect.GateServiceHandler) *Server {
	return &Server{
		cfg: cfg,
		h:   h,
	}
}

type Server struct {
	cfg Config
	h   gatev1connect.GateServiceHandler
}

func (s *Server) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("starting api service", "bind", s.cfg.Bind)

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle(gatev1connect.NewGateServiceHandler(s.h, connect.WithInterceptors(otelInterceptor)))

	hs := newHTTPServer(s.cfg.Bind, mux, ctx)

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

// newHTTPServer builds the API server: HTTP/1.1 plus cleartext HTTP/2 (h2c,
// prior knowledge), which is what gRPC/Connect clients expect from a
// non-TLS endpoint. The x/net/http2/h2c wrapper used to provide this; it is
// deprecated in favour of http.Server.Protocols.
func newHTTPServer(bind string, handler http.Handler, baseCtx context.Context) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Addr:              bind,
		Handler:           handler,
		Protocols:         protocols,
		ReadTimeout:       time.Second * 5,
		ReadHeaderTimeout: time.Second * 5,
		WriteTimeout:      time.Second * 10,
		IdleTimeout:       time.Second * 30,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
}

func ignoreClosed(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
