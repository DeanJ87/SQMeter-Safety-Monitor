package sqmclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
)

func newFakeServer(sensors sqmclient.SensorsResponse, status sqmclient.StatusResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/sensors":
			json.NewEncoder(w).Encode(sensors)
		case "/api/status":
			json.NewEncoder(w).Encode(status)
		default:
			http.NotFound(w, r)
		}
	}))
}

func clearSensors() sqmclient.SensorsResponse {
	return sqmclient.SensorsResponse{
		SkyQuality: sqmclient.SkyQuality{SQM: 21.5, Bortle: 2.0},
		Environment: sqmclient.Environment{
			Temperature: 12.4, Humidity: 72.1, Dewpoint: 7.8,
		},
		CloudConditions: sqmclient.CloudConditions{
			CloudCoverPercent: 5.0, Description: "Clear",
		},
	}
}

func TestFetchSensors_OK(t *testing.T) {
	srv := newFakeServer(clearSensors(), sqmclient.StatusResponse{})
	defer srv.Close()

	c := sqmclient.New(srv.URL)
	sensors, err := c.FetchSensors(context.Background())
	if err != nil {
		t.Fatalf("FetchSensors: %v", err)
	}
	if sensors.SkyQuality.SQM != 21.5 {
		t.Errorf("SQM: want 21.5, got %v", sensors.SkyQuality.SQM)
	}
}

func TestFetchSensors_Unreachable(t *testing.T) {
	c := sqmclient.New("http://127.0.0.1:1") // nothing listening
	_, err := c.FetchSensors(context.Background())
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestFetchSensors_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	c := sqmclient.New(srv.URL)
	_, err := c.FetchSensors(context.Background())
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestFetchSensors_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := sqmclient.New(srv.URL)
	_, err := c.FetchSensors(context.Background())
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestFetchStatus_OK(t *testing.T) {
	status := sqmclient.StatusResponse{Uptime: 3600, RSSI: -62, Version: "0.0.1"}
	srv := newFakeServer(clearSensors(), status)
	defer srv.Close()

	c := sqmclient.New(srv.URL)
	s, err := c.FetchStatus(context.Background())
	if err != nil {
		t.Fatalf("FetchStatus: %v", err)
	}
	if s.Uptime != 3600 {
		t.Errorf("Uptime: want 3600, got %d", s.Uptime)
	}
}
