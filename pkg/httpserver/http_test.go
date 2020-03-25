package httpserver

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/klog"
)

func init() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Set("v", "9")
}

func TestNewServer(t *testing.T) {
	tt := []struct {
		name          string
		listenAddr    string
		uriToResponse map[string]string
		server        *Server
	}{
		{
			name:          "unset uriToResponse should init into an empty map",
			listenAddr:    "localhost",
			uriToResponse: nil,
			server: &Server{
				listeningAddr: "localhost",
				uriToResponse: map[string]string{},
			},
		},
		{
			name:       "init from existing uriToResponse",
			listenAddr: "localhost",
			uriToResponse: map[string]string{
				"foo": "bar",
			},
			server: &Server{
				listeningAddr: "localhost",
				uriToResponse: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(tc.listenAddr, tc.uriToResponse)

			if tc.server.server.Addr != s.listeningAddr {
				t.Errorf("expected %q, got %q", tc.server.server.Addr, s.listeningAddr)
			}

			if !reflect.DeepEqual(tc.server.uriToResponse, s.uriToResponse) {
				t.Errorf(cmp.Diff(tc.server.uriToResponse, s.uriToResponse))
			}
		})
	}
}

func TestParseData(t *testing.T) {
	tt := []struct {
		name          string
		data          []byte
		uriToResponse map[string]string
		expectedErr   error
	}{
		{
			name:          "nil data",
			data:          nil,
			uriToResponse: map[string]string{},
			expectedErr:   nil,
		},
		{
			name:          "empty data",
			data:          []byte{},
			uriToResponse: map[string]string{},
			expectedErr:   nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer("localhost", nil)
			err := s.ParseData(tc.data)

			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("expected error %v, got %v", tc.expectedErr, err)
			}

			if !reflect.DeepEqual(tc.uriToResponse, s.uriToResponse) {
				t.Error(cmp.Diff(tc.uriToResponse, s.uriToResponse))
			}
		})
	}
}

func TestRun(t *testing.T) {
	tt := []struct {
		name          string
		uriToResponse map[string]string
	}{
		{
			name: "standard",
			uriToResponse: map[string]string{
				"/foo":  "bar",
				"/foo2": "bar2",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			uriToResponseCopy := make(map[string]string)
			for k, v := range tc.uriToResponse {
				uriToResponseCopy[k] = v
			}

			s := NewServer("localhost:0", uriToResponseCopy)
			var wg sync.WaitGroup
			defer wg.Wait()
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.Run()
				if err != nil {
					t.Error(err)
				}
			}()
			defer func() {
				err := s.Shutdown(context.Background())
				if err != nil {
					t.Error(err)
				}
			}()

			_, _ = fmt.Printf("Waiting for server to start..\n")
			err := s.WaitForConnect(context.Background(), 100*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Printf("Server to started.\n")

			addr := s.getListeningAddr()
			for uri, expectedData := range tc.uriToResponse {
				t.Run(uri, func(t *testing.T) {
					req, err := http.NewRequest("GET", "http://"+addr+uri, nil)
					if err != nil {
						t.Fatal(err)
					}
					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						err := resp.Body.Close()
						if err != nil {
							t.Error(err)
						}
					}()

					if resp.StatusCode != http.StatusOK {
						t.Fatalf("returned status code: %d (%s)", resp.StatusCode, resp.Status)
					}

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						t.Fatal(err)
					}

					receivedData := string(body)
					if expectedData != receivedData {
						t.Error(cmp.Diff(expectedData, receivedData))
					}
				})
			}
		})
	}
}
