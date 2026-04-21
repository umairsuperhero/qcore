package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// fakeStore implements Store with canned responses. Each method is a
// small function value so tests can rewire behavior per-case without
// subclassing.
type fakeStore struct {
	list     func(ctx context.Context, page, limit int, search string) ([]subscriber.Subscriber, int64, error)
	get      func(ctx context.Context, imsi string) (*subscriber.Subscriber, error)
	create   func(ctx context.Context, sub *subscriber.Subscriber) error
	update   func(ctx context.Context, imsi string, updates *subscriber.Subscriber) error
	del      func(ctx context.Context, imsi string) error
	genAV    func(ctx context.Context, imsi string) (*subscriber.AuthVector, error)
	importFn func(ctx context.Context, r io.Reader) (int, error)
	exportFn func(ctx context.Context, w io.Writer) error
}

func (f *fakeStore) ListSubscribers(ctx context.Context, page, limit int, search string) ([]subscriber.Subscriber, int64, error) {
	return f.list(ctx, page, limit, search)
}
func (f *fakeStore) GetSubscriber(ctx context.Context, imsi string) (*subscriber.Subscriber, error) {
	return f.get(ctx, imsi)
}
func (f *fakeStore) CreateSubscriber(ctx context.Context, sub *subscriber.Subscriber) error {
	return f.create(ctx, sub)
}
func (f *fakeStore) UpdateSubscriber(ctx context.Context, imsi string, updates *subscriber.Subscriber) error {
	return f.update(ctx, imsi, updates)
}
func (f *fakeStore) DeleteSubscriber(ctx context.Context, imsi string) error {
	return f.del(ctx, imsi)
}
func (f *fakeStore) GenerateAuthVector(ctx context.Context, imsi string) (*subscriber.AuthVector, error) {
	return f.genAV(ctx, imsi)
}
func (f *fakeStore) ImportCSV(ctx context.Context, r io.Reader) (int, error) {
	return f.importFn(ctx, r)
}
func (f *fakeStore) ExportCSV(ctx context.Context, w io.Writer) error {
	return f.exportFn(ctx, w)
}

func newTestServer(t *testing.T, store *fakeStore, health HealthCheckFunc) *httptest.Server {
	t.Helper()
	log := logger.New("error", "console")
	api := NewAPI(store, health, log, nil)
	return httptest.NewServer(api.Router())
}

func decodeResponse(t *testing.T, resp *http.Response) APIResponse {
	t.Helper()
	var body APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	return body
}

func TestHealthCheck(t *testing.T) {
	t.Run("ok when pinger returns nil", func(t *testing.T) {
		srv := newTestServer(t, &fakeStore{}, func(context.Context) error { return nil })
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/api/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("503 when pinger returns error", func(t *testing.T) {
		srv := newTestServer(t, &fakeStore{}, func(context.Context) error { return errors.New("db down") })
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/api/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("want 503, got %d", resp.StatusCode)
		}
	})

	t.Run("ok when no pinger configured", func(t *testing.T) {
		srv := newTestServer(t, &fakeStore{}, nil)
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/api/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("want 200, got %d", resp.StatusCode)
		}
	})
}

func TestListSubscribers(t *testing.T) {
	t.Run("happy path with pagination", func(t *testing.T) {
		store := &fakeStore{
			list: func(_ context.Context, page, limit int, search string) ([]subscriber.Subscriber, int64, error) {
				if page != 2 || limit != 5 || search != "foo" {
					t.Errorf("unexpected args: page=%d limit=%d search=%q", page, limit, search)
				}
				return []subscriber.Subscriber{{IMSI: "001010000000001"}}, 42, nil
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/subscribers?page=2&limit=5&search=foo")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
		body := decodeResponse(t, resp)
		if body.Total != 42 || body.Page != 2 || body.Limit != 5 {
			t.Errorf("metadata mismatch: %+v", body)
		}
	})

	t.Run("defaults to page=1 limit=20", func(t *testing.T) {
		var gotPage, gotLimit int
		store := &fakeStore{
			list: func(_ context.Context, page, limit int, _ string) ([]subscriber.Subscriber, int64, error) {
				gotPage = page
				gotLimit = limit
				return nil, 0, nil
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/subscribers")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if gotPage != 1 || gotLimit != 20 {
			t.Errorf("defaults not applied: page=%d limit=%d", gotPage, gotLimit)
		}
	})

	t.Run("500 on store error", func(t *testing.T) {
		store := &fakeStore{
			list: func(context.Context, int, int, string) ([]subscriber.Subscriber, int64, error) {
				return nil, 0, errors.New("db exploded")
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/subscribers")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("want 500, got %d", resp.StatusCode)
		}
	})
}

func TestGetSubscriber(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"ok", nil, http.StatusOK},
		{"not found → 404", errors.New("subscriber 001 not found"), http.StatusNotFound},
		{"other error → 500", errors.New("db boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				get: func(_ context.Context, imsi string) (*subscriber.Subscriber, error) {
					if tc.err != nil {
						return nil, tc.err
					}
					return &subscriber.Subscriber{IMSI: imsi}, nil
				},
			}
			srv := newTestServer(t, store, nil)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/api/v1/subscribers/001010000000001")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Errorf("want %d, got %d", tc.status, resp.StatusCode)
			}
		})
	}
}

func TestCreateSubscriber(t *testing.T) {
	validBody := `{"imsi":"001010000000001","ki":"465b5ce8b199b49faa5f0a2ee238a6bc","opc":"cd63cb71954a9f4e48a5994e37a02baf"}`

	cases := []struct {
		name   string
		body   string
		err    error
		status int
	}{
		{"201 on success", validBody, nil, http.StatusCreated},
		{"400 on malformed JSON", `{not json`, nil, http.StatusBadRequest},
		{"409 on already exists", validBody, errors.New("subscriber with IMSI 001010000000001 already exists"), http.StatusConflict},
		{"400 on validation error", validBody, errors.New("validation: IMSI must be 15 digits"), http.StatusBadRequest},
		{"500 on other error", validBody, errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				create: func(context.Context, *subscriber.Subscriber) error { return tc.err },
			}
			srv := newTestServer(t, store, nil)
			defer srv.Close()

			resp, err := http.Post(srv.URL+"/api/v1/subscribers", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Errorf("want %d, got %d", tc.status, resp.StatusCode)
			}
		})
	}
}

func TestUpdateSubscriber(t *testing.T) {
	validBody := `{"msisdn":"15551234567"}`

	t.Run("200 on success + refetches subscriber", func(t *testing.T) {
		updateCalled, getCalled := false, false
		store := &fakeStore{
			update: func(_ context.Context, imsi string, _ *subscriber.Subscriber) error {
				updateCalled = true
				if imsi != "001010000000001" {
					t.Errorf("wrong imsi: %s", imsi)
				}
				return nil
			},
			get: func(_ context.Context, imsi string) (*subscriber.Subscriber, error) {
				getCalled = true
				return &subscriber.Subscriber{IMSI: imsi, MSISDN: "15551234567"}, nil
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/subscribers/001010000000001", strings.NewReader(validBody))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("want 200, got %d", resp.StatusCode)
		}
		if !updateCalled || !getCalled {
			t.Errorf("update=%v get=%v, want both true", updateCalled, getCalled)
		}
	})

	t.Run("404 when subscriber not found", func(t *testing.T) {
		store := &fakeStore{
			update: func(context.Context, string, *subscriber.Subscriber) error {
				return errors.New("subscriber 001 not found")
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/subscribers/001010000000001", strings.NewReader(validBody))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("400 on malformed JSON", func(t *testing.T) {
		srv := newTestServer(t, &fakeStore{}, nil)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/subscribers/001010000000001", strings.NewReader(`{not json`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestDeleteSubscriber(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"200 on success", nil, http.StatusOK},
		{"404 when not found", errors.New("subscriber 001 not found"), http.StatusNotFound},
		{"500 on other error", errors.New("db boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				del: func(context.Context, string) error { return tc.err },
			}
			srv := newTestServer(t, store, nil)
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/subscribers/001010000000001", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.status {
				t.Errorf("want %d, got %d", tc.status, resp.StatusCode)
			}
		})
	}
}

func TestGenerateAuthVector(t *testing.T) {
	t.Run("200 returns the vector", func(t *testing.T) {
		want := &subscriber.AuthVector{RAND: "aabb", AUTN: "ccdd", XRES: "eeff", KASME: "1234"}
		store := &fakeStore{
			genAV: func(context.Context, string) (*subscriber.AuthVector, error) { return want, nil },
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		resp, err := http.Post(srv.URL+"/api/v1/subscribers/001010000000001/auth-vector", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}

		// The vector lives inside APIResponse.Data — re-encode and re-decode.
		body := decodeResponse(t, resp)
		raw, _ := json.Marshal(body.Data)
		var got subscriber.AuthVector
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decoding vector: %v", err)
		}
		if got != *want {
			t.Errorf("vector mismatch:\n got %+v\nwant %+v", got, *want)
		}
	})

	t.Run("404 when subscriber missing", func(t *testing.T) {
		store := &fakeStore{
			genAV: func(context.Context, string) (*subscriber.AuthVector, error) {
				return nil, errors.New("subscriber 001 not found")
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		resp, err := http.Post(srv.URL+"/api/v1/subscribers/001010000000001/auth-vector", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})
}

func TestImportCSV(t *testing.T) {
	t.Run("200 with imported count", func(t *testing.T) {
		store := &fakeStore{
			importFn: func(_ context.Context, r io.Reader) (int, error) {
				data, _ := io.ReadAll(r)
				if !strings.Contains(string(data), "imsi") {
					t.Errorf("expected CSV payload to reach store, got %q", data)
				}
				return 3, nil
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		body, contentType := buildMultipart(t, "file", "subs.csv",
			"imsi,ki,opc\n001010000000001,465b5ce8b199b49faa5f0a2ee238a6bc,cd63cb71954a9f4e48a5994e37a02baf\n")

		resp, err := http.Post(srv.URL+"/api/v1/subscribers/import", contentType, body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
		decoded := decodeResponse(t, resp)
		raw, _ := json.Marshal(decoded.Data)
		var out map[string]int
		_ = json.Unmarshal(raw, &out)
		if out["imported"] != 3 {
			t.Errorf("imported count: got %d, want 3", out["imported"])
		}
	})

	t.Run("400 on missing file field", func(t *testing.T) {
		srv := newTestServer(t, &fakeStore{}, nil)
		defer srv.Close()

		// Empty multipart body → no "file" field → FormFile returns error.
		body, contentType := buildMultipart(t, "other", "x.txt", "x")
		resp, err := http.Post(srv.URL+"/api/v1/subscribers/import", contentType, body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("400 on store error", func(t *testing.T) {
		store := &fakeStore{
			importFn: func(context.Context, io.Reader) (int, error) {
				return 0, errors.New("row 2: malformed")
			},
		}
		srv := newTestServer(t, store, nil)
		defer srv.Close()

		body, contentType := buildMultipart(t, "file", "subs.csv", "imsi\n001")
		resp, err := http.Post(srv.URL+"/api/v1/subscribers/import", contentType, body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestExportCSV(t *testing.T) {
	store := &fakeStore{
		exportFn: func(_ context.Context, w io.Writer) error {
			_, err := fmt.Fprintf(w, "imsi,msisdn\n001010000000001,15551234567\n")
			return err
		},
	}
	srv := newTestServer(t, store, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/subscribers/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type: got %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "subscribers.csv") {
		t.Errorf("Content-Disposition missing filename: %q", cd)
	}
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "imsi,msisdn") {
		t.Errorf("CSV body missing header: %q", data)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	// Panicking store must not crash the server — middleware turns it into 500.
	store := &fakeStore{
		list: func(context.Context, int, int, string) ([]subscriber.Subscriber, int64, error) {
			panic("boom")
		},
	}
	srv := newTestServer(t, store, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/subscribers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
}

// Compile-time check: *subscriber.Service satisfies Store.
var _ Store = (*subscriber.Service)(nil)

func buildMultipart(t *testing.T, field, filename, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}
