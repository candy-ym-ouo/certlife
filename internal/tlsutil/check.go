// Package tlsutil verifies deployed TLS certificates.
package tlsutil
import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"certlife/internal/model"
)
func ParsePEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil { return nil, fmt.Errorf("no PEM certificate found") }
	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil { return nil, fmt.Errorf("parse certificate: %w", e) }
	return cert, nil
}
func FingerprintSHA256(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	encoded := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 { parts = append(parts, encoded[i:i+2]) }
	return strings.Join(parts, ":")
}
func VerifyTLS(host string, port int, wantSerial string, wantSANs []string) (model.VerificationResult, error) {
	result := model.VerificationResult{CheckedAt: time.Now().UTC()}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, e := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if e != nil { result.Error = e.Error(); return result, e }
	defer conn.Close()
	result.HandshakeOK = true
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 { e = fmt.Errorf("server returned no certificate"); result.Error = e.Error(); return result, e }
	cert := state.PeerCertificates[0]
	want := strings.TrimLeft(strings.ToUpper(wantSerial), "0")
	got := strings.TrimLeft(strings.ToUpper(cert.SerialNumber.Text(16)), "0")
	result.SerialOK = want == "" || want == got
	result.ValidityOK = time.Now().After(cert.NotBefore) && time.Now().Before(cert.NotAfter)
	result.Fingerprint = FingerprintSHA256(cert)
	result.SANsOK = true
	for _, name := range wantSANs {
		if e = cert.VerifyHostname(name); e != nil { result.SANsOK = false; break }
	}
	if !result.OK() { e = fmt.Errorf("verification failed: serial=%t sans=%t validity=%t", result.SerialOK, result.SANsOK, result.ValidityOK); result.Error = e.Error(); return result, e }
	return result, nil
}
