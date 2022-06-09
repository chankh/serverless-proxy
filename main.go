package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/api/idtoken"
)

func health(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ok:%s", time.Now())
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := realMain(ctx); err != nil {
		cancel()

		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func realMain(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/_hc", health)

	// Initialize the proxy handler
	ph := proxyHandler{ctx: ctx}
	mux.Handle("/", ph)

	// Create server
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	// Start server in background.
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("Listening on port: 8080\n")

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	// Wait for stop
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		fmt.Fprint(os.Stderr, "\nserver is shutting down...\n")
	}

	// Attempt graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}
	return nil
}

type proxyHandler struct {
	ctx context.Context
}

func (ph proxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requestUrl := req.URL.Path
	fmt.Printf("requestUrl = %s\n", requestUrl)

	// Get the best token source. Cloud Run expects the audience parameter to be
	// the URL of the service.
	url := "https:/" + requestUrl

	// client is a http.Client that automatically adds an "Authorization" header
	// to any requests made.
	client, err := idtoken.NewClient(ph.ctx, url)
	if err != nil {
		fmt.Printf("idtoken.NewClient: %v\n", err)
		failRequest(w, http.StatusUnauthorized)
		return
	}

	fmt.Printf("Forwarding request to: %s\n", url)

	r, err := http.NewRequest(req.Method, url, req.Body)
	if err != nil {
		fmt.Printf("http.NewRequest: %v\n", err)
		failRequest(w, http.StatusInternalServerError)
		return
	}
	r.Header = req.Header
	resp, err := client.Do(r)
	if err != nil {
		fmt.Printf("client.Get: %v\n", err)
		failRequest(w, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		fmt.Printf("io.Copy: %v\n", err)
		failRequest(w, http.StatusInternalServerError)
		return
	}
}

func failRequest(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
