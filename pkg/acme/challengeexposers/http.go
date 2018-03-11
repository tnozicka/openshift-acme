package challengeexposers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/crypto/acme"
)

const (
	ShutdownTimeout = 3 * time.Second
)

type Http01 struct {
	urlToToken   map[string]string
	domainToUrls map[string][]string
	mutex        sync.RWMutex
	Addr         string
}

var _ Interface = &Http01{}

func NewHttp01(ctx context.Context, addr string) (*Http01, error) {
	s := &Http01{
		urlToToken:   make(map[string]string),
		domainToUrls: make(map[string][]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handler)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	// if you don't specify addr (e.g. port) we need to find to which it was bound so e.g. tests can use it
	s.Addr = listener.Addr().String()
	glog.Infof("Http-01: server listening on http://%s/", s.Addr)

	go func() {
		<-ctx.Done()
		glog.Infof("Http-01: stopping server listening on http://%s/", s.Addr)
		ctx, cancel := context.WithTimeout(ctx, ShutdownTimeout)
		defer cancel()
		server.Shutdown(ctx)
	}()

	go server.Serve(listener)

	return s, nil
}

func (h *Http01) getKey(url string) (string, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	key, found := h.urlToToken[url]
	return key, found
}

func (h *Http01) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	uri := strings.Split(r.Host, ":")[0] + r.URL.String()
	key, found := h.getKey(uri)
	glog.V(4).Infof("url = '%s'; found = '%t'", uri, found)
	if found {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, key)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	return
}

func getHttp01Uri(a *acme.Client, domain string, token string) string {
	return domain + a.HTTP01ChallengePath(token)
}

func (h *Http01) Expose(a *acme.Client, domain string, token string) error {
	if domain == "" {
		return errors.New("domain can't be empty")
	}

	url := getHttp01Uri(a, domain, token)
	key, err := a.HTTP01ChallengeResponse(token)
	if err != nil {
		return err
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.urlToToken[url] = key
	h.domainToUrls[domain] = append(h.domainToUrls[domain], url)

	return nil
}

func (h *Http01) Remove(domain string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	urls, found := h.domainToUrls[domain]
	if !found {
		return fmt.Errorf("domain %q was not found", domain)
	}

	if len(urls) < 1 {
		return fmt.Errorf("internal error, no register url for domain %q", domain)
	}

	for _, url := range urls {
		delete(h.urlToToken, url)
	}
	delete(h.domainToUrls, domain)

	return nil
}
