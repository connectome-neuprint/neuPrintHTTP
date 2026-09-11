package main

// This driver and its control surface exist only in the root test binary.
// Build with go test -c, then select TestPredeployDriver and supply its flags.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

var predeployConfig = flag.String("predeploy-config", "", "test driver config JSON (no listener when omitted)")
var predeployDatasets = flag.String("predeploy-datasets", "", "comma-separated exact fixture dataset/version names")
var predeployCert = flag.String("predeploy-cert", "", "write the generated loopback trust certificate here")

type predeployQuery struct {
	Dataset  string `json:"dataset"`
	Query    string `json:"query"`
	ReadOnly bool   `json:"read_only"`
	Accessor string `json:"accessor"`
}

type predeployStore struct {
	storage.NoStore
	mu      sync.Mutex
	queries []predeployQuery
}

var _ storage.Store = (*predeployStore)(nil)
var _ storage.Cypher = (*predeployCypher)(nil)

func (s *predeployStore) GetVersion() (string, error) { return "0.5.0", nil }
func (s *predeployStore) GetDataset(dataset string) (storage.Cypher, error) {
	for _, name := range s.Datasets {
		if name == dataset {
			return &predeployCypher{s, dataset, "GetDataset"}, nil
		}
	}
	return nil, fmt.Errorf("unknown fixture dataset %q", dataset)
}
func (s *predeployStore) GetMain(datasets ...string) storage.Cypher {
	dataset := ""
	if len(datasets) != 0 {
		dataset = datasets[0]
	} else if len(s.Datasets) != 0 {
		dataset = s.Datasets[0]
	}
	return &predeployCypher{s, dataset, "GetMain"}
}
func (s *predeployStore) snapshot() []predeployQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]predeployQuery{}, s.queries...)
}

type predeployCypher struct {
	store    *predeployStore
	dataset  string
	accessor string
}

func (c *predeployCypher) CypherRequest(query string, readonly bool) (storage.CypherResult, error) {
	c.store.mu.Lock()
	c.store.queries = append(c.store.queries, predeployQuery{c.dataset, query, readonly, c.accessor})
	c.store.mu.Unlock()
	// Production route setup warms metadata caches through GetMain. Keep those
	// calls visible, but return empty metadata just like NoStore. Custom queries
	// go through GetDataset, so their execution is counted independently.
	if c.accessor == "GetMain" {
		return storage.CypherResult{Columns: []string{}, Data: [][]interface{}{}}, nil
	}
	return storage.CypherResult{
		Columns: []string{"fixture", "dataset"},
		Data:    [][]interface{}{{"predeploy-ok", c.dataset}},
	}, nil
}
func (c *predeployCypher) StartTrans() (storage.CypherTransaction, error) {
	return nil, errors.New("fixture transactions are not supported")
}

func predeployCertificate(path string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "neuPrint predeploy fixture"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:        true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, certPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func runPredeployDriver(ctx context.Context, readyWriter io.Writer) error {
	if *predeployCert == "" || *predeployDatasets == "" {
		return errors.New("predeploy-cert and predeploy-datasets are required")
	}
	options, err := config.LoadConfig(*predeployConfig)
	if err != nil {
		return err
	}
	dsgURL, err := url.Parse(options.DSGUrl)
	if err != nil || dsgURL.Scheme != "http" || dsgURL.Hostname() != "127.0.0.1" {
		return errors.New("predeploy dsg-url must be an http://127.0.0.1 loopback address")
	}
	if options.DSGCacheTTL != 1 {
		return errors.New("predeploy dsg-cache-ttl must be 1 second")
	}
	store := &predeployStore{NoStore: storage.NoStore{Datasets: strings.Split(*predeployDatasets, ",")}}
	e, _, client, err := newServer(options, store, io.Discard)
	if err != nil {
		return err
	}
	e.Logger.SetOutput(os.Stderr)
	// Native DSG remains real, with a bound shorter than Python's request deadline.
	client.SetHTTPClient(&http.Client{Timeout: 2 * time.Second})
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Before(func() {
				if email, ok := c.Get("email").(string); ok {
					c.Response().Header().Set("X-Predeploy-Identity", email)
				}
			})
			return next(c)
		}
	})
	e.GET("/__predeploy/state", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{"queries": store.snapshot(), "ttl_seconds": options.DSGCacheTTL})
	})
	cert, err := predeployCertificate(*predeployCert)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("predeploy loopback listen: %w", err)
	}
	defer listener.Close()
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	server := &http.Server{Handler: e, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(tlsListener) }()
	if err := json.NewEncoder(readyWriter).Encode(map[string]interface{}{
		"address": "https://" + listener.Addr().String(), "ttl_seconds": options.DSGCacheTTL,
	}); err != nil {
		server.Close()
		return err
	}
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
			return err
		}
	}
	return nil
}

func TestPredeployDriver(t *testing.T) {
	if *predeployConfig == "" {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Package initialization logs belong on stderr; stdout is the readiness
	// protocol. This affects only the explicitly invoked test driver process.
	readyWriter := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = readyWriter }()
	if err := runPredeployDriver(ctx, readyWriter); err != nil {
		fmt.Fprintln(os.Stderr, "predeploy driver:", err)
		t.Fatal("predeploy driver startup or serving failed; see stderr")
	}
}

func TestPredeployStoreCountsActualQueries(t *testing.T) {
	store := &predeployStore{NoStore: storage.NoStore{Datasets: []string{"fixture:v1"}}}
	if _, err := store.GetDataset("missing"); err == nil {
		t.Fatal("unknown dataset accepted")
	}
	cypher, err := store.GetDataset("fixture:v1")
	if err != nil {
		t.Fatal(err)
	}
	result, err := cypher.CypherRequest("RETURN 1", true)
	if err != nil || result.Data[0][0] != "predeploy-ok" {
		t.Fatalf("query result: %v, %v", result, err)
	}
	_, err = store.GetMain("fixture:v1").CypherRequest("RETURN 2", false)
	if err != nil {
		t.Fatal(err)
	}
	queries := store.snapshot()
	if len(queries) != 2 || queries[0] != (predeployQuery{"fixture:v1", "RETURN 1", true, "GetDataset"}) || queries[1].ReadOnly || queries[1].Accessor != "GetMain" {
		t.Fatalf("incorrect backend counts: %+v", queries)
	}
}
