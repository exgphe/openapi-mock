package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type httpServer struct {
	server        *http.Server
	httpListener  net.Listener
	httpsListener net.Listener
}

func New(httpsPort uint16, httpPort uint16, handler http.Handler, logger *log.Logger) (Server, error) {
	server := &httpServer{
		server: &http.Server{
			//Addr:           fmt.Sprintf(":%d", httpsPort),
			Handler:        handler,
			ErrorLog:       logger,
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   0, // Supports SSE
			IdleTimeout:    30 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}
	httpsListener, err := net.Listen("tcp", fmt.Sprintf(":%d", httpsPort))
	if err != nil {
		return nil, err
	}
	server.httpsListener = httpsListener
	httpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", httpPort))
	if err != nil {
		return nil, err
	}
	server.httpListener = httpListener
	return server, nil
}

func (httpServer *httpServer) Run() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	done := make(chan bool)
	doneHttp := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		httpServer.server.ErrorLog.Printf("%s - Shutdown signal received...\n", hostname)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		httpServer.server.SetKeepAlivesEnabled(false)

		if err := httpServer.server.Shutdown(ctx); err != nil {
			httpServer.server.ErrorLog.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		_ = httpServer.httpListener.Close()
		_ = httpServer.httpsListener.Close()
		close(done)
		close(doneHttp)
	}()
	httpServer.server.ErrorLog.Printf("%s - Starting HTTPS server on %v", hostname, httpServer.httpsListener.Addr().String())

	go func() {
		time.Sleep(1 * time.Second)
		httpServer.server.ErrorLog.Printf("%s - Starting HTTP server on %v", hostname, httpServer.httpListener.Addr().String())
		if err := httpServer.server.Serve(httpServer.httpListener); errors.Is(err, http.ErrServerClosed) {
			httpServer.server.ErrorLog.Fatalf("Could not listen on %s: %v\n", httpServer.httpListener.Addr().String(), err)
		}
	}()

	if err := httpServer.server.ServeTLS(httpServer.httpsListener, "server.crt", "server.key"); errors.Is(err, http.ErrServerClosed) {
		httpServer.server.ErrorLog.Fatalf("Could not listen on %s: %v\n", httpServer.httpsListener.Addr().String(), err)
	}

	<-done
	httpServer.server.ErrorLog.Printf("%s - Server has been gracefully stopped.\n", hostname)

	return nil
}
