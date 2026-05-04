package alpaca_test

// ConformU bad-path and transaction-ID casing tests.
//
// ConformU probes malformed Alpaca URLs and expects a non-200 response that
// does not contain an HTML body.  It also sends Alpaca transaction parameters
// with non-canonical casing and expects the IDs to be echoed back correctly.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sqmeter-ascom-alpaca/internal/alpaca"
)

// newMux returns a fully registered ServeMux for the given handler.
func newMux(h *alpaca.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// serve issues a GET request against mux and returns the recorder.
func serveGET(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// assertNon200AndNotHTML checks that the response is a client-error status and
// that the body does not look like an HTML page.
func assertNon200AndNotHTML(t *testing.T, label string, w *httptest.ResponseRecorder) {
	t.Helper()
	code := w.Code
	if code < 400 || code >= 600 {
		t.Errorf("%s: want 4xx status, got %d", label, code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		t.Errorf("%s: response body looks like HTML (status %d)", label, code)
	}
}

// ---------- bad-path tests ---------------------------------------------------

func TestBadPath_WrongDeviceTypeCaps(t *testing.T) {
	// /api/v1/SAFETYMONITOR/0/description — capitalised device type
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/SAFETYMONITOR/0/description")
	assertNon200AndNotHTML(t, "SAFETYMONITOR caps", w)
}

func TestBadPath_BadDeviceType(t *testing.T) {
	// /api/v1/baddevicetype/0/description — completely wrong device type
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/baddevicetype/0/description")
	assertNon200AndNotHTML(t, "baddevicetype", w)
}

func TestBadPath_NegativeDeviceNumber(t *testing.T) {
	// /api/v1/safetymonitor/-1/description
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/safetymonitor/-1/description")
	assertNon200AndNotHTML(t, "device number -1", w)
}

func TestBadPath_LargeDeviceNumber(t *testing.T) {
	// /api/v1/safetymonitor/99999/description — out-of-range device number
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/safetymonitor/99999/description")
	assertNon200AndNotHTML(t, "device number 99999", w)
}

func TestBadPath_AlphaDeviceNumber(t *testing.T) {
	// /api/v1/safetymonitor/A/description — non-numeric device number
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/safetymonitor/A/description")
	assertNon200AndNotHTML(t, "device number A", w)
}

func TestBadPath_TruncatedMethod(t *testing.T) {
	// /api/v1/safetymonitor/0/descrip — typo in method name
	mux := newMux(newTestHandler(true, safeState()))
	w := serveGET(mux, "/api/v1/safetymonitor/0/descrip")
	assertNon200AndNotHTML(t, "descrip typo", w)
}

// ---------- ClientTransactionID casing tests ---------------------------------

func TestClientTransactionID_LowerCase(t *testing.T) {
	// ConformU may send "clienttransactionid" in lowercase
	h := newTestHandler(true, safeState())
	mux := newMux(h)
	req := httptest.NewRequest(http.MethodGet, "/management/apiversions?clienttransactionid=77", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var env struct {
		ClientTransactionID uint32 `json:"ClientTransactionID"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.ClientTransactionID != 77 {
		t.Errorf("ClientTransactionID: want 77, got %d", env.ClientTransactionID)
	}
}

func TestClientTransactionID_UpperCase(t *testing.T) {
	// ConformU may also send "CLIENTTRANSACTIONID" in uppercase
	h := newTestHandler(true, safeState())
	mux := newMux(h)
	req := httptest.NewRequest(http.MethodGet, "/management/apiversions?CLIENTTRANSACTIONID=99", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var env struct {
		ClientTransactionID uint32 `json:"ClientTransactionID"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.ClientTransactionID != 99 {
		t.Errorf("ClientTransactionID: want 99, got %d", env.ClientTransactionID)
	}
}

func TestClientTransactionID_MixedCase_PUT(t *testing.T) {
	// Case-insensitive parsing for PUT form bodies
	h := newTestHandler(true, safeState())
	mux := newMux(h)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/connect",
		strings.NewReader("ClientID=1&clienttransactionid=55"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.VoidResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ClientTransactionID != 55 {
		t.Errorf("ClientTransactionID: want 55, got %d", resp.ClientTransactionID)
	}
}
