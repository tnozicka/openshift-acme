package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

type Server struct {
	uriToResponse map[string]string

	server http.Server

	listeningAddr      string
	listeningAddrMutex sync.Mutex
}

func NewServer(listenAddr string, uriToResponse map[string]string) *Server {
	if uriToResponse == nil {
		uriToResponse = make(map[string]string)
	}
	return &Server{
		uriToResponse: uriToResponse,

		server: http.Server{
			Addr: listenAddr,
		},
	}
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	// host, _, err := net.SplitHostPort(r.Host)
	// if err != nil {
	// 	klog.Error(err)
	// 	host = r.Host
	// }
	// uri := host + r.URL.String()
	uri := r.URL.String()
	response, found := s.uriToResponse[uri]
	klog.V(4).Infof("URI %q %sfound", uri, func() string {
		if !found {
			return "not "
		}
		return ""
	}())

	if found {
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprint(w, response)
		if err != nil {
			klog.Errorf("unable to write response")
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)

	return
}

func (s *Server) ParseData(data []byte) error {
	lines := strings.Split(string(data), "\n")
	klog.Infof("Parsing %d line(s)", len(lines))
	for n, l := range lines {
		if len(strings.TrimSpace(l)) == 0 {
			continue
		}

		parts := strings.SplitN(l, " ", 2)
		if len(parts) != 2 {
			// don't print the content as it contains secret data
			return fmt.Errorf("can't parse line %d", n)
		}
		uri := parts[0]
		response := parts[1]
		s.uriToResponse[uri] = response
	}

	return nil
}

func (s *Server) setListeningAddr(addr string) {
	s.listeningAddrMutex.Lock()
	defer s.listeningAddrMutex.Unlock()
	s.listeningAddr = addr
}

func (s *Server) getListeningAddr() string {
	s.listeningAddrMutex.Lock()
	defer s.listeningAddrMutex.Unlock()
	return s.listeningAddr
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handler)
	s.server.Handler = mux

	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	// if you don't specify addr (e.g. port) we need to find to which it was bound so e.g. tests can use it
	s.setListeningAddr(listener.Addr().String())
	klog.V(1).Infof("Http-01: server listening on http://%s/", s.listeningAddr)

	err = s.server.Serve(listener)
	if err == http.ErrServerClosed {
		klog.Infof("Server closed gracefully")
		return nil
	}

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	klog.Infof("Shutting down server...")
	defer klog.Infof("Server shut down")

	return s.server.Shutdown(ctx)
}

func (s *Server) WaitForConnect(ctx context.Context, pollInterval time.Duration) error {
	return wait.PollImmediateUntil(pollInterval, func() (done bool, err error) {
		_, err = net.DialTimeout("tcp", s.getListeningAddr(), 3*time.Second)
		if err == nil {
			return true, nil
		}
		return false, nil
	}, ctx.Done())
}
