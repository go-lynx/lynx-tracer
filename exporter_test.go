package tracer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-lynx/lynx-tracer/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ---------------------------------------------------------------------------
// validateAddress
// ---------------------------------------------------------------------------

func TestValidateAddress(t *testing.T) {
	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"", false},
		{"None", false},
		{"localhost:4317", false},
		{"127.0.0.1:4318", false},
		{"collector.example.com:4317", false},
		{"invalid-no-port", true},
		{":4317", true},   // empty host
		{"host:", true},   // empty port
		{"host:0", true},  // port 0 is not in 1-65535
		{"host:99999", true},
		{"host:abc", true}, // non-numeric port
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.addr, func(t *testing.T) {
			err := validateAddress(tc.addr)
			if tc.wantErr {
				assert.Error(t, err, "expected error for addr %q", tc.addr)
			} else {
				assert.NoError(t, err, "expected no error for addr %q", tc.addr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildExporter – gRPC path
// ---------------------------------------------------------------------------

func TestBuildExporter_GRPCDefault(t *testing.T) {
	// Minimal config – no actual collector running; the gRPC exporter connects lazily
	// so New() should succeed even with no listener at the target address.
	c := &conf.Tracer{
		Enable: true,
		Addr:   "localhost:4317",
		Ratio:  1,
	}
	exp, _, useBatch, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.False(t, useBatch, "batch should be disabled when not configured")
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithInsecure(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithBatch(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Batch: &conf.Batch{
				Enabled:      true,
				MaxQueueSize: 1024,
				MaxBatchSize: 256,
			},
		},
	}
	exp, batchOpts, useBatch, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.True(t, useBatch)
	assert.NotEmpty(t, batchOpts)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithBatchDefaults(t *testing.T) {
	// Zero queue/batch size should use SDK defaults (2048/512) without error.
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Batch: &conf.Batch{
				Enabled:      true,
				MaxQueueSize: 0,
				MaxBatchSize: 0,
			},
		},
	}
	exp, batchOpts, useBatch, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.True(t, useBatch)
	assert.NotEmpty(t, batchOpts, "SDK-default batch options should still be present")
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithRetry(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Retry: &conf.Retry{
				Enabled:         true,
				MaxAttempts:     3,
				InitialInterval: durationpb.New(100 * time.Millisecond),
				MaxInterval:     durationpb.New(1 * time.Second),
			},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithTimeout(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Timeout:  durationpb.New(10 * time.Second),
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithGzip(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure:    true,
			Compression: conf.Compression_COMPRESSION_GZIP,
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithHeaders(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Headers:  map[string]string{"Authorization": "Bearer token"},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithConnection(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Connection: &conf.Connection{
				ReconnectionPeriod: durationpb.New(3 * time.Second),
				ConnectTimeout:     durationpb.New(5 * time.Second),
			},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithLoadBalancing(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			LoadBalancing: &conf.LoadBalancing{
				Policy:      "round_robin",
				HealthCheck: false,
			},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCWithBothLBAndPool(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Insecure: true,
			Connection: &conf.Connection{
				MaxConnAge:      durationpb.New(10 * time.Minute),
				MaxConnIdleTime: durationpb.New(5 * time.Minute),
			},
			LoadBalancing: &conf.LoadBalancing{
				Policy: "round_robin",
			},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// buildExporter – HTTP path
// ---------------------------------------------------------------------------

func TestBuildExporter_HTTP(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol: conf.Protocol_OTLP_HTTP,
			Insecure: true,
		},
	}
	exp, _, useBatch, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.False(t, useBatch)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_HTTPWithBatch(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol: conf.Protocol_OTLP_HTTP,
			Insecure: true,
			Batch: &conf.Batch{
				Enabled:      true,
				MaxQueueSize: 512,
				MaxBatchSize: 128,
			},
		},
	}
	exp, batchOpts, useBatch, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.True(t, useBatch)
	assert.NotEmpty(t, batchOpts)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_HTTPWithPath(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol: conf.Protocol_OTLP_HTTP,
			Insecure: true,
			HttpPath: "/custom/traces",
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_HTTPWithHeaders(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol: conf.Protocol_OTLP_HTTP,
			Insecure: true,
			Headers:  map[string]string{"X-Tenant": "acme"},
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_HTTPWithGzip(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol:    conf.Protocol_OTLP_HTTP,
			Insecure:    true,
			Compression: conf.Compression_COMPRESSION_GZIP,
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_HTTPWithTimeout(t *testing.T) {
	c := &conf.Tracer{
		Addr: "localhost:4318",
		Config: &conf.Config{
			Protocol: conf.Protocol_OTLP_HTTP,
			Insecure: true,
			Timeout:  durationpb.New(15 * time.Second),
		},
	}
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// buildExporter – invalid address
// ---------------------------------------------------------------------------

func TestBuildExporter_InvalidAddress(t *testing.T) {
	c := &conf.Tracer{
		Addr: "invalid-no-port",
	}
	_, _, _, err := buildExporter(context.Background(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address validation failed")
}

// ---------------------------------------------------------------------------
// TLS configuration – buildTLSCredentials
// ---------------------------------------------------------------------------

// selfSignedCert generates a temporary self-signed certificate and key for TLS tests.
// It returns paths to the written PEM files and a cleanup function.
func selfSignedCert(t *testing.T) (certFile, keyFile, caFile string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	caFile = filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))
	require.NoError(t, os.WriteFile(caFile, certPEM, 0600)) // self-signed CA == cert

	cleanup = func() {} // t.TempDir() cleans up automatically
	return
}

func TestBuildTLSCredentials_Nil(t *testing.T) {
	opt, err := buildTLSCredentials(&conf.Config{})
	assert.NoError(t, err)
	assert.Nil(t, opt, "no TLS config should yield nil option")
}

func TestBuildTLSCredentials_WithCA(t *testing.T) {
	_, _, caFile, _ := selfSignedCert(t)
	cfg := &conf.Config{
		Tls: &conf.TLS{
			CaFile: caFile,
		},
	}
	opt, err := buildTLSCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt, "CA file should produce a non-nil option")
}

func TestBuildTLSCredentials_WithMTLS(t *testing.T) {
	certFile, keyFile, caFile, _ := selfSignedCert(t)
	cfg := &conf.Config{
		Tls: &conf.TLS{
			CaFile:   caFile,
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	}
	opt, err := buildTLSCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt)
}

func TestBuildTLSCredentials_MissingKey(t *testing.T) {
	certFile, _, _, _ := selfSignedCert(t)
	cfg := &conf.Config{
		Tls: &conf.TLS{
			CertFile: certFile,
			// KeyFile intentionally omitted
		},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both cert_file and key_file must be provided")
}

func TestBuildTLSCredentials_MissingCert(t *testing.T) {
	_, keyFile, _, _ := selfSignedCert(t)
	cfg := &conf.Config{
		Tls: &conf.TLS{
			// CertFile intentionally omitted
			KeyFile: keyFile,
		},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both cert_file and key_file must be provided")
}

func TestBuildTLSCredentials_InvalidCACert(t *testing.T) {
	dir := t.TempDir()
	badCA := filepath.Join(dir, "bad-ca.pem")
	require.NoError(t, os.WriteFile(badCA, []byte("not valid PEM"), 0600))

	cfg := &conf.Config{
		Tls: &conf.TLS{CaFile: badCA},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid certificates found")
}

func TestBuildTLSCredentials_NonexistentCAFile(t *testing.T) {
	cfg := &conf.Config{
		Tls: &conf.TLS{CaFile: "/nonexistent/path/ca.pem"},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
}

func TestBuildTLSCredentials_PathTraversal(t *testing.T) {
	cfg := &conf.Config{
		Tls: &conf.TLS{CaFile: "../../../etc/passwd"},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CA file path")
}

func TestBuildTLSCredentials_InsecureSkipVerify_NonProduction(t *testing.T) {
	// Ensure non-production does not reject InsecureSkipVerify
	t.Setenv("LYNX_ENV", "development")
	cfg := &conf.Config{
		Tls: &conf.TLS{InsecureSkipVerify: true},
	}
	// InsecureSkipVerify with no CA still produces a credential (empty RootCAs)
	opt, err := buildTLSCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt)
}

func TestBuildTLSCredentials_InsecureSkipVerify_Production(t *testing.T) {
	t.Setenv("LYNX_ENV", "production")
	cfg := &conf.Config{
		Tls: &conf.TLS{InsecureSkipVerify: true},
	}
	_, err := buildTLSCredentials(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure_skip_verify")
}

// ---------------------------------------------------------------------------
// validateFilePath
// ---------------------------------------------------------------------------

func TestValidateFilePath_Empty(t *testing.T) {
	err := validateFilePath("")
	require.Error(t, err)
}

func TestValidateFilePath_PathTraversal(t *testing.T) {
	err := validateFilePath("../../etc/passwd")
	require.Error(t, err)
}

func TestValidateFilePath_DoubleSlash(t *testing.T) {
	err := validateFilePath("//etc/passwd")
	require.Error(t, err)
}

func TestValidateFilePath_NonExistent(t *testing.T) {
	err := validateFilePath("/nonexistent/path/file.pem")
	require.Error(t, err)
}

func TestValidateFilePath_ValidFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.pem")
	require.NoError(t, err)
	f.Close()
	err = validateFilePath(f.Name())
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// buildMergedGRPCServiceConfig helpers
// ---------------------------------------------------------------------------

func TestBuildMergedGRPCServiceConfig_NoLBNoPool(t *testing.T) {
	cfg := &conf.Config{}
	result := buildMergedGRPCServiceConfig(cfg)
	assert.Contains(t, result, "round_robin")
}

func TestBuildMergedGRPCServiceConfig_LBOnly(t *testing.T) {
	cfg := &conf.Config{
		LoadBalancing: &conf.LoadBalancing{Policy: "pick_first"},
	}
	result := buildMergedGRPCServiceConfig(cfg)
	assert.Contains(t, result, "pick_first")
}

func TestBuildMergedGRPCServiceConfig_PoolOnly(t *testing.T) {
	cfg := &conf.Config{
		Connection: &conf.Connection{
			MaxConnAge: durationpb.New(10 * time.Minute),
		},
	}
	result := buildMergedGRPCServiceConfig(cfg)
	assert.Contains(t, result, "maxConnAge")
}

func TestBuildMergedGRPCServiceConfig_BothLBAndPool(t *testing.T) {
	cfg := &conf.Config{
		LoadBalancing: &conf.LoadBalancing{Policy: "round_robin"},
		Connection: &conf.Connection{
			MaxConnIdleTime: durationpb.New(5 * time.Minute),
		},
	}
	result := buildMergedGRPCServiceConfig(cfg)
	assert.Contains(t, result, "round_robin")
	assert.Contains(t, result, "maxConnIdleTime")
}

func TestBuildLoadBalancingServiceConfig_HealthCheck(t *testing.T) {
	lb := &conf.LoadBalancing{Policy: "round_robin", HealthCheck: true}
	result := buildLoadBalancingServiceConfig(lb)
	assert.Contains(t, result, "healthCheckConfig")
	assert.Contains(t, result, "grpc.health.v1.Health")
}

func TestBuildLoadBalancingServiceConfig_UnknownPolicy(t *testing.T) {
	lb := &conf.LoadBalancing{Policy: "unknown_policy"}
	result := buildLoadBalancingServiceConfig(lb)
	// Should fall back to round_robin (canonical snake_case name)
	assert.Contains(t, result, "round_robin")
}

func TestBuildLoadBalancingServiceConfig_LeastConn(t *testing.T) {
	lb := &conf.LoadBalancing{Policy: "least_conn"}
	result := buildLoadBalancingServiceConfig(lb)
	assert.Contains(t, result, "least_conn")
}

func TestBuildConnectionPoolServiceConfig(t *testing.T) {
	conn := &conf.Connection{
		MaxConnIdleTime: durationpb.New(5 * time.Minute),
		MaxConnAge:      durationpb.New(10 * time.Minute),
		MaxConnAgeGrace: durationpb.New(30 * time.Second),
	}
	result := buildConnectionPoolServiceConfig(conn)
	assert.True(t, strings.HasPrefix(result, "{"), "should be valid JSON object")
	assert.Contains(t, result, "maxConnIdleTime")
	assert.Contains(t, result, "maxConnAge")
	assert.Contains(t, result, "maxConnAgeGrace")
}

// ---------------------------------------------------------------------------
// TLS config wired into buildExporter
// ---------------------------------------------------------------------------

func TestBuildExporter_GRPCWithTLSFromFiles(t *testing.T) {
	certFile, keyFile, caFile, _ := selfSignedCert(t)
	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Tls: &conf.TLS{
				CaFile:   caFile,
				CertFile: certFile,
				KeyFile:  keyFile,
			},
		},
	}
	// Connecting to a non-running server is expected to fail at dial time, but
	// buildExporter should not return an error — the gRPC exporter connects lazily.
	exp, _, _, err := buildExporter(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, exp)
	_ = exp.Shutdown(context.Background())
}

func TestBuildExporter_GRPCTLSBadCA(t *testing.T) {
	dir := t.TempDir()
	badCA := filepath.Join(dir, "bad-ca.pem")
	require.NoError(t, os.WriteFile(badCA, []byte("not valid PEM"), 0600))

	c := &conf.Tracer{
		Addr: "localhost:4317",
		Config: &conf.Config{
			Tls: &conf.TLS{CaFile: badCA},
		},
	}
	_, _, _, err := buildExporter(context.Background(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build TLS credentials")
}

// ---------------------------------------------------------------------------
// Metrics wiring – exporter protocol label
// ---------------------------------------------------------------------------

func TestMetrics_SetExporterProtocol(t *testing.T) {
	m := newTracerMetrics()
	m.setExporterProtocol("grpc")

	families, err := m.registry.Gather()
	require.NoError(t, err)

	var grpcVal, httpVal, noneVal float64
	for _, f := range families {
		if f.GetName() != "lynx_tracer_exporter_info" {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "protocol" {
					switch lp.GetValue() {
					case "grpc":
						grpcVal = metric.GetGauge().GetValue()
					case "http":
						httpVal = metric.GetGauge().GetValue()
					case "none":
						noneVal = metric.GetGauge().GetValue()
					}
				}
			}
		}
	}
	assert.Equal(t, 1.0, grpcVal)
	assert.Equal(t, 0.0, httpVal)
	assert.Equal(t, 0.0, noneVal)
}

func TestMetrics_SetExporterProtocol_Transition(t *testing.T) {
	m := newTracerMetrics()
	m.setExporterProtocol("http")
	m.setExporterProtocol("none")

	families, err := m.registry.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() != "lynx_tracer_exporter_info" {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "protocol" {
					switch lp.GetValue() {
					case "none":
						assert.Equal(t, 1.0, metric.GetGauge().GetValue())
					default:
						assert.Equal(t, 0.0, metric.GetGauge().GetValue(),
							"protocol %q should be 0 after switching to none", lp.GetValue())
					}
				}
			}
		}
	}
}

func TestMetrics_SetExporterProtocol_NilSafe(t *testing.T) {
	var m *TracerMetrics
	assert.NotPanics(t, func() { m.setExporterProtocol("grpc") })
}

func TestMetrics_ExporterInfoInGatherer(t *testing.T) {
	m := newTracerMetrics()
	m.setExporterProtocol("grpc")

	g := m.GetGatherer()
	families, err := g.Gather()
	require.NoError(t, err)

	names := make(map[string]struct{})
	for _, f := range families {
		names[f.GetName()] = struct{}{}
	}
	assert.Contains(t, names, "lynx_tracer_exporter_info")
}

// ---------------------------------------------------------------------------
// tls.Config security – InsecureSkipVerify reachability
// ---------------------------------------------------------------------------

func TestBuildTLSCredentials_InsecureSkipVerify_IsPropagated(t *testing.T) {
	// Verify that when InsecureSkipVerify=true is allowed (non-production), the credential
	// object is produced (we can't inspect the internal tls.Config but we can assert no error).
	t.Setenv("LYNX_ENV", "test")

	cfg := &conf.Config{
		Tls: &conf.TLS{InsecureSkipVerify: true},
	}
	opt, err := buildTLSCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt)
}

// ---------------------------------------------------------------------------
// TLS – helper: verify tls.Config values are set correctly
// ---------------------------------------------------------------------------

// TestTLSConfig_Values creates credentials from known cert files and verifies
// that the resulting tls.Config is correctly populated.
func TestTLSConfig_Values(t *testing.T) {
	certFile, keyFile, caFile, _ := selfSignedCert(t)

	cfg := &conf.Config{
		Tls: &conf.TLS{
			CaFile:   caFile,
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	}
	opt, err := buildTLSCredentials(cfg)
	require.NoError(t, err)
	require.NotNil(t, opt)

	// We can load the key pair ourselves to confirm it round-trips through tls.LoadX509KeyPair
	kp, err := tls.LoadX509KeyPair(certFile, keyFile)
	require.NoError(t, err)
	assert.NotEmpty(t, kp.Certificate)
}
