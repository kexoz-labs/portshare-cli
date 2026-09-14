package tunnel

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"
)

type RequestRecord struct {
	ID          string              `json:"id"`
	Timestamp   time.Time           `json:"timestamp"`
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	ReqHeaders  map[string][]string `json:"reqHeaders"`
	ReqBody     []byte              `json:"reqBody,omitempty"`
	RespStatus  int                 `json:"respStatus"`
	RespHeaders map[string][]string `json:"respHeaders"`
	RespBody    []byte              `json:"respBody,omitempty"`
	DurationMs  int64               `json:"durationMs"`
}

const maxRequests = 100

var (
	inspectorMutex sync.RWMutex
	capturedReqs   = make([]*RequestRecord, 0, maxRequests)
	listeners      []chan struct{}
	
	globalDo       func(*http.Request) (*http.Response, error)
	globalPort     int
)

func SetReplayTarget(port int, do func(*http.Request) (*http.Response, error)) {
	inspectorMutex.Lock()
	defer inspectorMutex.Unlock()
	globalPort = port
	globalDo = do
}

func GetReplayTarget() (int, func(*http.Request) (*http.Response, error)) {
	inspectorMutex.RLock()
	defer inspectorMutex.RUnlock()
	return globalPort, globalDo
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GetCapturedRequests() []*RequestRecord {
	inspectorMutex.RLock()
	defer inspectorMutex.RUnlock()
	// Return a copy of the slice
	cp := make([]*RequestRecord, len(capturedReqs))
	copy(cp, capturedReqs)
	return cp
}

func GetRequest(id string) *RequestRecord {
	inspectorMutex.RLock()
	defer inspectorMutex.RUnlock()
	for _, req := range capturedReqs {
		if req.ID == id {
			return req
		}
	}
	return nil
}

func notifyListeners() {
	inspectorMutex.Lock()
	defer inspectorMutex.Unlock()
	for _, l := range listeners {
		select {
		case l <- struct{}{}:
		default:
		}
	}
}

// Intercept executes the HTTP request, capturing the request and response details.
func Intercept(req *http.Request, do func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	record := &RequestRecord{
		ID:         generateID(),
		Timestamp:  time.Now(),
		Method:     req.Method,
		Path:       req.URL.Path,
		ReqHeaders: req.Header.Clone(),
	}
	if req.URL.RawQuery != "" {
		record.Path += "?" + req.URL.RawQuery
	}

	// Capture request body
	if req.Body != nil {
		reqBytes, _ := io.ReadAll(req.Body)
		record.ReqBody = reqBytes
		req.Body = io.NopCloser(bytes.NewBuffer(reqBytes))
	}

	// Add to UI immediately as pending
	inspectorMutex.Lock()
	if len(capturedReqs) >= maxRequests {
		// Remove oldest
		capturedReqs = capturedReqs[1:]
	}
	capturedReqs = append(capturedReqs, record)
	inspectorMutex.Unlock()
	
	go notifyListeners()

	start := time.Now()
	resp, err := do(req)
	record.DurationMs = time.Since(start).Milliseconds()

	if err != nil {
		record.RespStatus = 502
		go notifyListeners()
		return resp, err
	}

	record.RespStatus = resp.StatusCode
	record.RespHeaders = resp.Header.Clone()

	// Capture response body
	if resp.Body != nil {
		respBytes, _ := io.ReadAll(resp.Body)
		// Don't capture excessively large bodies (e.g. video streams)
		if len(respBytes) < 5*1024*1024 {
			record.RespBody = respBytes
		} else {
			record.RespBody = []byte("<Response body too large to display>")
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(respBytes))
	}

	go notifyListeners()
	return resp, err
}
