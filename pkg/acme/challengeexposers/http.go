package challengeexposers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/go-playground/log"
	"golang.org/x/crypto/acme"
)

type Http01 struct {
	logger  log.LeveledLogger
	mapping map[string]string
	mutex   sync.RWMutex
	Addr    string
}

func (h *Http01) getKey(url string) (key string, found bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	key, found = h.mapping[url]
	return
}

func (h *Http01) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	uri := strings.Split(r.Host, ":")[0] + r.URL.String()
	key, found := h.getKey(uri)
	log.Debugf("url = '%s'; found = '%t'", uri, found)
	if found {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, key)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	return
}

func NewHttp01(context context.Context, addr string, logger log.LeveledLogger) (h *Http01, err error) {
	h = &Http01{
		logger:  logger,
		mapping: make(map[string]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handler)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}

	// if you don't specify addr (e.g. port) we need to find to which it was bound so e.g. tests can use it
	h.Addr = listener.Addr().String()
	h.logger.Infof("Http-01: server listening on http://%s/", h.Addr)

	// TODO: rewrite this to use Shutdown method once we have Go 1.8
	go func() {
		<-context.Done()
		h.logger.Infof("Http-01: stopping server listening on http://%s/", h.Addr)
		listener.Close()
	}()

	go func() {
		h.logger.Error(server.Serve(listener))
	}()

	return
}

func getHttp01Uri(a *acme.Client, domain string, token string) (url string) {
	url = domain + a.HTTP01ChallengePath(token)
	return
}

func (h *Http01) Expose(a *acme.Client, domain string, token string) (err error) {
	if domain == "" {
		return errors.New("domain can't be empty")
	}

	url := getHttp01Uri(a, domain, token)
	key, err := a.HTTP01ChallengeResponse(token)
	if err != nil {
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()
	// TODO: consider checking if there is already a value with same key
	h.mapping[url] = key

	return
}

func (h *Http01) Remove(a *acme.Client, domain string, token string) error {
	url := getHttp01Uri(a, domain, token)
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// TODO: consider checking if there is already a value with same key
	delete(h.mapping, url)

	return nil
}
