package untypedclient

import (
	"fmt"
	"io"
	"sync"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/runtime/serializer/json"
	"k8s.io/client-go/rest"
)

type StreamWatcher struct {
	sync.Mutex
	source  io.ReadCloser
	result  chan []byte
	stopped bool
}

func NewStreamWatcher(s io.ReadCloser) *StreamWatcher {
	sw := &StreamWatcher{
		source: s,
		result: make(chan []byte),
	}
	go sw.receive()
	return sw
}

func (sw *StreamWatcher) ResultChan() <-chan []byte {
	return sw.result
}

func (sw *StreamWatcher) Stop() {
	sw.Lock()
	defer sw.Unlock()
	if !sw.stopped {
		sw.stopped = true
		sw.source.Close()
	}
}

func (sw *StreamWatcher) stopping() bool {
	sw.Lock()
	defer sw.Unlock()
	return sw.stopped
}

func (sw *StreamWatcher) receive() {
	defer close(sw.result)
	defer sw.Stop()
	buffer := make([]byte, 100*1024)
	for {
		n, err := sw.source.Read(buffer)
		if err != nil {
			// Ignore expected error.
			if sw.stopping() {
				return
			}

			switch err {
			case io.EOF:
				// watch closed normally
			default:
				sw.source.Close()
				//panic(err)  // TODO: investigate and remove
				return
			}

			return
		}
		result := make([]byte, n)
		copy(result, buffer)
		sw.result <- result
	}
}

func Watch(rc rest.Interface, path string) (sw *StreamWatcher, err error) {
	req := rc.Get()
	req.RequestURI(path)
	body, err := req.Stream()
	if err != nil {
		return nil, fmt.Errorf("watch failed: returned body: %s", err)
	}

	framer := json.Framer.NewFrameReader(body)

	sw = NewStreamWatcher(framer)

	return
}

func Get(rc rest.Interface, path string) (body []byte, err error) {
	req := rc.Get()
	req.RequestURI(path)
	body, err = req.DoRaw()
	if err != nil {
		return
	}

	return
}

func Post(rc rest.Interface, path string, payload []byte) (body []byte, err error) {
	req := rc.Post()
	req.RequestURI(path)
	req.Body(payload)
	body, err = req.DoRaw()
	if err != nil {
		return
	}

	return
}

func Put(rc rest.Interface, path string, payload []byte) (body []byte, err error) {
	req := rc.Put()
	req.RequestURI(path)
	req.Body(payload)
	body, err = req.DoRaw()
	if err != nil {
		return
	}

	return
}

func Patch(rc rest.Interface, path string, payload []byte) (body []byte, err error) {
	req := rc.Patch(api.StrategicMergePatchType)
	req.RequestURI(path)
	req.Body(payload)
	body, err = req.DoRaw()
	if err != nil {
		return
	}

	return
}

func Delete(rc rest.Interface, path string, payload []byte) (body []byte, err error) {
	req := rc.Delete()
	req.RequestURI(path)
	req.Body(payload)
	body, err = req.DoRaw()
	if err != nil {
		return
	}

	return
}
