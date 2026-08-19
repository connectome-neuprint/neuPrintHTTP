package keyvalue

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/connectome-neuprint/neuPrintHTTP/secure"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

type keyValueRoundTripFunc func(*http.Request) (*http.Response, error)

func (f keyValueRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type keyValueTestStore struct {
	datasets map[string]interface{}
	getCalls int
}

func (s *keyValueTestStore) GetVersion() (string, error)                  { return "1", nil }
func (s *keyValueTestStore) GetDatabase() (string, string, error)         { return "", "", nil }
func (s *keyValueTestStore) GetDatasets() (map[string]interface{}, error) { return s.datasets, nil }
func (s *keyValueTestStore) GetType() string                              { return "keyvalue" }
func (s *keyValueTestStore) GetInstance() string                          { return "owned-instance" }
func (s *keyValueTestStore) GetMain(...string) storage.Cypher             { return nil }
func (s *keyValueTestStore) GetDataset(string) (storage.Cypher, error)    { return nil, nil }
func (s *keyValueTestStore) GetStores() []storage.SimpleStore             { return []storage.SimpleStore{s} }
func (s *keyValueTestStore) GetInstances() map[string]storage.SimpleStore {
	return map[string]storage.SimpleStore{"owned-instance": s}
}
func (s *keyValueTestStore) GetTypes() map[string][]storage.SimpleStore            { return nil }
func (s *keyValueTestStore) FindStore(string, string) (storage.SimpleStore, error) { return s, nil }
func (s *keyValueTestStore) Get([]byte) ([]byte, error) {
	s.getCalls++
	return []byte("value"), nil
}
func (s *keyValueTestStore) Set([]byte, []byte) error { return nil }

func TestRawKeyValueGetUsesOwningDatasetAndFailsClosed(t *testing.T) {
	tests := []struct {
		name         string
		datasets     map[string]interface{}
		decision     string
		wantCode     int
		wantGetCalls int
		wantDSGCalls int
	}{
		{"no owning dataset", map[string]interface{}{}, "allow", http.StatusUnauthorized, 0, 0},
		{"closed owning dataset", map[string]interface{}{"owned-dataset": nil}, "deny", http.StatusUnauthorized, 0, 1},
		{"public owning dataset", map[string]interface{}{"owned-dataset": nil}, "allow", http.StatusOK, 1, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := secure.NewDSGClient("http://dsg.test", 300, "neuprint")
			client.SetHTTPClient(&http.Client{Transport: keyValueRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if !bytes.Contains(body, []byte(`"name":"owned-dataset"`)) {
					t.Fatalf("authorize request did not use owning dataset: %s", body)
				}
				response := `{"entries":[{"name":"owned-dataset","decision":"` + tc.decision + `","roles":["view"]}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(response)),
				}, nil
			})})
			store := &keyValueTestStore{datasets: tc.datasets}
			handler := masterAPI{Store: store}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/raw/keyvalue/key/owned-instance/key", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPath("/api/raw/keyvalue/key/:instance/:key")
			c.SetParamNames("instance", "key")
			c.SetParamValues("owned-instance", "key")
			c.Set("dsg_client", client)

			err := handler.getKV(c)
			if tc.wantCode == http.StatusOK {
				if err != nil || rec.Code != http.StatusOK {
					t.Fatalf("err=%v status=%d", err, rec.Code)
				}
			} else {
				httpErr, ok := err.(*echo.HTTPError)
				if !ok || httpErr.Code != tc.wantCode {
					t.Fatalf("err=%v, want HTTP %d", err, tc.wantCode)
				}
			}
			if store.getCalls != tc.wantGetCalls || calls != tc.wantDSGCalls {
				t.Fatalf("store gets=%d DSG calls=%d, want %d/%d", store.getCalls, calls, tc.wantGetCalls, tc.wantDSGCalls)
			}
		})
	}
}
