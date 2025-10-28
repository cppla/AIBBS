package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	DEFAULT_READ_TIMEOUT   = 60 * time.Second
	DEFAULT_WRITE_TIMEOUT  = DEFAULT_READ_TIMEOUT
	GRACEFUL_ENVIRON_KEY   = "IS_GRACEFUL"
	GRACEFUL_ENVIRON_VALUE = GRACEFUL_ENVIRON_KEY + "=1"
	GRACEFUL_LISTENER_FD   = 3
)

// Server wraps http.Server to support graceful shutdown and restart.
type Server struct {
	*http.Server

	listener     net.Listener
	isGraceful   bool
	signalChan   chan os.Signal
	shutdownChan chan struct{}
}

// NewServer creates a Server with timeouts and handler.
func NewServer(addr string, handler http.Handler, readTimeout, writeTimeout time.Duration) *Server {
	isGraceful := os.Getenv(GRACEFUL_ENVIRON_KEY) != ""
	return &Server{
		Server: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		isGraceful:   isGraceful,
		signalChan:   make(chan os.Signal, 1),
		shutdownChan: make(chan struct{}),
	}
}

// ListenAndServe starts serving on tcp and handles signals.
func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := srv.getNetListener(addr)
	if err != nil {
		return err
	}
	srv.listener = ln
	return srv.serve()
}

// ListenAndServeTLS starts TLS server with graceful features.
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}

	cfg := &tls.Config{}
	if srv.TLSConfig != nil {
		*cfg = *srv.TLSConfig
	}
	if cfg.NextProtos == nil {
		cfg.NextProtos = []string{"http/1.1"}
	}
	var err error
	cfg.Certificates = make([]tls.Certificate, 1)
	cfg.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	ln, err := srv.getNetListener(addr)
	if err != nil {
		return err
	}
	srv.listener = tls.NewListener(ln, cfg)
	return srv.serve()
}

func (srv *Server) serve() error {
	go srv.handleSignals()
	err := srv.Server.Serve(srv.listener)
	// Wait until Shutdown finished
	<-srv.shutdownChan
	return err
}

func (srv *Server) getNetListener(addr string) (net.Listener, error) {
	if srv.isGraceful {
		file := os.NewFile(GRACEFUL_LISTENER_FD, "")
		ln, err := net.FileListener(file)
		if err != nil {
			return nil, fmt.Errorf("net.FileListener error: %w", err)
		}
		return ln, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("net.Listen error: %w", err)
	}
	return ln, nil
}

func (srv *Server) handleSignals() {
	signal.Notify(
		srv.signalChan,
		syscall.SIGTERM,
		syscall.SIGUSR2,
	)

	for sig := range srv.signalChan {
		switch sig {
		case syscall.SIGTERM:
			Sugar.Info("received SIGTERM, graceful shutting down HTTP server")
			srv.shutdownHTTPServer()
		case syscall.SIGUSR2:
			Sugar.Info("received SIGUSR2, graceful restarting HTTP server")
			if pid, err := srv.startNewProcess(); err != nil {
				Sugar.Errorf("start new process failed: %v, continue serving", err)
			} else {
				Sugar.Infof("start new process succeeded, new pid=%d", pid)
				Sugar.Info("closing old HTTP server after new one started")
				srv.shutdownHTTPServer()
			}
		}
	}
}

func (srv *Server) shutdownHTTPServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		Sugar.Errorf("HTTP server shutdown error: %v", err)
	} else {
		Sugar.Info("HTTP server shutdown success")
	}
	close(srv.shutdownChan)
}

// start new process to handle HTTP connections
func (srv *Server) startNewProcess() (uintptr, error) {
	// obtain listener fd
	tcpLn, ok := srv.listener.(*net.TCPListener)
	if !ok {
		return 0, fmt.Errorf("listener is not *net.TCPListener")
	}
	file, err := tcpLn.File()
	if err != nil {
		return 0, fmt.Errorf("get listener file: %w", err)
	}
	listenerFd := file.Fd()

	// set graceful env
	envs := []string{}
	for _, e := range os.Environ() {
		if e != GRACEFUL_ENVIRON_VALUE {
			envs = append(envs, e)
		}
	}
	envs = append(envs, GRACEFUL_ENVIRON_VALUE)

	attr := &syscall.ProcAttr{
		Env:   envs,
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd(), listenerFd},
	}
	pid, err := syscall.ForkExec(os.Args[0], os.Args, attr)
	if err != nil {
		return 0, fmt.Errorf("forkexec: %w", err)
	}
	return uintptr(pid), nil
}

// GraceServer starts an HTTP server with graceful capabilities.
func GraceServer(addr string, handler http.Handler) error {
	return NewServer(addr, handler, DEFAULT_READ_TIMEOUT, DEFAULT_WRITE_TIMEOUT).ListenAndServe()
}

// GraceServerTLS starts an HTTPS server with graceful capabilities.
func GraceServerTLS(addr, certFile, keyFile string, handler http.Handler) error {
	return NewServer(addr, handler, DEFAULT_READ_TIMEOUT, DEFAULT_WRITE_TIMEOUT).ListenAndServeTLS(certFile, keyFile)
}
