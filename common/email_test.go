package common

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeSMTPServer struct {
	listener          net.Listener
	host              string
	port              int
	cert              tls.Certificate
	advertiseSTARTTLS bool
	authMechanisms    []string
	rejectRcptWith    string // non-empty: reply this line to every RCPT TO
	messages          chan string
	authCommands      chan string
	startTLSCommands  chan string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	return newFakeSMTPServerWithSTARTTLSAdvertisement(t, true)
}

func newFakeSMTPServerWithSTARTTLSAdvertisement(t *testing.T, advertiseSTARTTLS bool) *fakeSMTPServer {
	t.Helper()

	cert, err := newTestTLSCertificate()
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener:          listener,
		host:              host,
		port:              port,
		cert:              cert,
		advertiseSTARTTLS: advertiseSTARTTLS,
		authMechanisms:    []string{"PLAIN", "LOGIN"},
		messages:          make(chan string, 1),
		authCommands:      make(chan string, 1),
		startTLSCommands:  make(chan string, 1),
	}
	go server.serve()
	return server
}

func newFakeImplicitTLSSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()

	cert, err := newTestTLSCertificate()
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener:          tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}}),
		host:              host,
		port:              port,
		cert:              cert,
		advertiseSTARTTLS: false,
		authMechanisms:    []string{"PLAIN", "LOGIN"},
		messages:          make(chan string, 1),
		authCommands:      make(chan string, 1),
		startTLSCommands:  make(chan string, 1),
	}
	go server.serve()
	return server
}

func (s *fakeSMTPServer) close() {
	_ = s.listener.Close()
}

func (s *fakeSMTPServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if err := writeSMTPLine(rw, "220 fake.smtp.local ESMTP"); err != nil {
		return
	}

	encrypted := false
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimRight(line, "\r\n")
		upperCommand := strings.ToUpper(command)

		switch {
		case strings.HasPrefix(upperCommand, "EHLO"):
			if err := writeSMTPLine(rw, "250-fake.smtp.local"); err != nil {
				return
			}
			if !encrypted && s.advertiseSTARTTLS {
				if err := writeSMTPLine(rw, "250-STARTTLS"); err != nil {
					return
				}
			}
			if len(s.authMechanisms) > 0 {
				if err := writeSMTPLine(rw, "250 AUTH "+strings.Join(s.authMechanisms, " ")); err != nil {
					return
				}
			} else if err := writeSMTPLine(rw, "250 8BITMIME"); err != nil {
				return
			}
		case upperCommand == "STARTTLS":
			if encrypted || !s.advertiseSTARTTLS {
				if err := writeSMTPLine(rw, "502 5.5.1 STARTTLS not supported"); err != nil {
					return
				}
				continue
			}
			select {
			case s.startTLSCommands <- command:
			default:
			}
			if err := writeSMTPLine(rw, "220 2.0.0 Ready to start TLS"); err != nil {
				return
			}
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
			encrypted = true
		case strings.HasPrefix(upperCommand, "AUTH"):
			select {
			case s.authCommands <- command:
			default:
			}
			if err := writeSMTPLine(rw, "235 2.7.0 Authentication successful"); err != nil {
				return
			}
		case strings.HasPrefix(upperCommand, "MAIL FROM:"):
			if err := writeSMTPLine(rw, "250 2.1.0 Sender OK"); err != nil {
				return
			}
		case strings.HasPrefix(upperCommand, "RCPT TO:"):
			reply := "250 2.1.5 Recipient OK"
			if s.rejectRcptWith != "" {
				reply = s.rejectRcptWith
			}
			if err := writeSMTPLine(rw, reply); err != nil {
				return
			}
		case upperCommand == "DATA":
			if err := writeSMTPLine(rw, "354 End data with <CR><LF>.<CR><LF>"); err != nil {
				return
			}
			var data strings.Builder
			for {
				dataLine, err := rw.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				data.WriteString(dataLine)
			}
			s.messages <- data.String()
			if err := writeSMTPLine(rw, "250 2.0.0 Queued"); err != nil {
				return
			}
		case upperCommand == "QUIT":
			_ = writeSMTPLine(rw, "221 2.0.0 Bye")
			return
		default:
			if err := writeSMTPLine(rw, "502 5.5.1 Command not implemented"); err != nil {
				return
			}
		}
	}
}

func writeSMTPLine(rw *bufio.ReadWriter, line string) error {
	_, err := rw.WriteString(line + "\r\n")
	if err != nil {
		return err
	}
	return rw.Flush()
}

func newTestTLSCertificate() (tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "aixinexchange01.aixin-chip.com",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"aixinexchange01", "aixinexchange01.aixin-chip.com"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func withSMTPSettings(t *testing.T) {
	t.Helper()
	originalSMTPServer := SMTPServer
	originalSMTPPort := SMTPPort
	originalSMTPSSLEnabled := SMTPSSLEnabled
	originalSMTPStartTLSEnabled := SMTPStartTLSEnabled
	originalSMTPInsecureSkipVerify := SMTPInsecureSkipVerify
	originalSMTPForceAuthLogin := SMTPForceAuthLogin
	originalSMTPAccount := SMTPAccount
	originalSMTPFrom := SMTPFrom
	originalSMTPToken := SMTPToken
	originalMarketingSMTPServer := MarketingSMTPServer
	originalMarketingSMTPPort := MarketingSMTPPort
	originalMarketingSMTPSSLEnabled := MarketingSMTPSSLEnabled
	originalMarketingSMTPStartTLSEnabled := MarketingSMTPStartTLSEnabled
	originalMarketingSMTPInsecureSkipVerify := MarketingSMTPInsecureSkipVerify
	originalMarketingSMTPForceAuthLogin := MarketingSMTPForceAuthLogin
	originalMarketingSMTPAccount := MarketingSMTPAccount
	originalMarketingSMTPFrom := MarketingSMTPFrom
	originalMarketingSMTPToken := MarketingSMTPToken
	originalSystemName := SystemName

	t.Cleanup(func() {
		SMTPServer = originalSMTPServer
		SMTPPort = originalSMTPPort
		SMTPSSLEnabled = originalSMTPSSLEnabled
		SMTPStartTLSEnabled = originalSMTPStartTLSEnabled
		SMTPInsecureSkipVerify = originalSMTPInsecureSkipVerify
		SMTPForceAuthLogin = originalSMTPForceAuthLogin
		SMTPAccount = originalSMTPAccount
		SMTPFrom = originalSMTPFrom
		SMTPToken = originalSMTPToken
		MarketingSMTPServer = originalMarketingSMTPServer
		MarketingSMTPPort = originalMarketingSMTPPort
		MarketingSMTPSSLEnabled = originalMarketingSMTPSSLEnabled
		MarketingSMTPStartTLSEnabled = originalMarketingSMTPStartTLSEnabled
		MarketingSMTPInsecureSkipVerify = originalMarketingSMTPInsecureSkipVerify
		MarketingSMTPForceAuthLogin = originalMarketingSMTPForceAuthLogin
		MarketingSMTPAccount = originalMarketingSMTPAccount
		MarketingSMTPFrom = originalMarketingSMTPFrom
		MarketingSMTPToken = originalMarketingSMTPToken
		SystemName = originalSystemName
	})
	MarketingSMTPServer = ""
	MarketingSMTPPort = 587
	MarketingSMTPSSLEnabled = false
	MarketingSMTPStartTLSEnabled = false
	MarketingSMTPInsecureSkipVerify = false
	MarketingSMTPForceAuthLogin = false
	MarketingSMTPAccount = ""
	MarketingSMTPFrom = ""
	MarketingSMTPToken = ""
}

func TestSendEmailUsesExplicitStartTLSWithInsecureCertificate(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case message := <-server.messages:
		require.Contains(t, message, "Subject: =?UTF-8?B?")
		require.Contains(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailExplicitStartTLSRequiresServerSupport(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.Error(t, err)
	require.Contains(t, err.Error(), "STARTTLS")
}

func TestSendEmailDoesNotAutoUpgradeWhenStartTLSDisabled(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, true)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.startTLSCommands:
		t.Fatalf("unexpected SMTP STARTTLS command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		require.Contains(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSMTPPlainAuthRejectsRemotePlaintextConnection(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = "smtp.example.com"
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	client, err := smtp.NewClient(conn, SMTPServer)
	require.NoError(t, err)

	err = client.Auth(AutoSMTPAuth(mainSMTPConfig()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unencrypted connection")

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}
}

func TestNewSMTPClientHonorsExplicitStartTLSWhenPortIs465(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = 465
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true

	client, err := newSMTPClient(mainSMTPConfig(), fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	defer client.Close()

	select {
	case command := <-server.startTLSCommands:
		require.Equal(t, "STARTTLS", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP STARTTLS")
	}
}

func TestNewSMTPClientKeepsImplicitTLSForLegacyPort465(t *testing.T) {
	server := newFakeImplicitTLSSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = 465
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = true

	client, err := newSMTPClient(mainSMTPConfig(), fmt.Sprintf("%s:%d", server.host, server.port))
	require.NoError(t, err)
	defer client.Close()
}

func TestSendEmailSkipsAuthWhenCredentialsAreEmpty(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = ""
	SMTPFrom = "sender@example.com"
	SMTPToken = ""
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		require.Contains(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailSkipsAuthWhenCredentialsAreIncomplete(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = ""
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		t.Fatalf("unexpected SMTP auth command: %s", command)
	default:
	}

	select {
	case message := <-server.messages:
		require.Contains(t, message, "<p>123456</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailUsesNTLMWhenServerOnlySupportsNTLM(t *testing.T) {
	server := newFakeSMTPServer(t)
	server.authMechanisms = []string{"NTLM"}
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "no-reply"
	SMTPFrom = "no-reply@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		require.True(t, strings.HasPrefix(command, "AUTH NTLM "), "unexpected auth command: %s", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP AUTH")
	}
}

func TestSendEmailUsesNTLMForMicrosoftAccountWhenServerOnlySupportsNTLM(t *testing.T) {
	server := newFakeSMTPServer(t)
	server.authMechanisms = []string{"NTLM"}
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = true
	SMTPForceAuthLogin = false
	SMTPAccount = "no-reply@contoso.onmicrosoft.com"
	SMTPFrom = "no-reply@contoso.onmicrosoft.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.NoError(t, err)

	select {
	case command := <-server.authCommands:
		require.True(t, strings.HasPrefix(command, "AUTH NTLM "), "unexpected auth command: %s", command)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP AUTH")
	}
}

// TestSendEmailRcptRejectionClassification guards the suppression-learning
// contract: only 5xx RCPT rejections that identify the recipient mailbox as
// invalid (enhanced status 5.1.x or a "no such user" phrase) may feed the
// suppression list. Policy/relay refusals (5.7.x — the signature of a
// misconfigured relay rejecting EVERY recipient), quota failures (552), and
// transient 4xx must never be learned as hard bounces.
func TestSendEmailRcptRejectionClassification(t *testing.T) {
	cases := []struct {
		name      string
		reply     string
		permanent bool
	}{
		{"invalid mailbox 5.1.1", "550 5.1.1 user unknown", true},
		{"invalid mailbox phrase without enhanced code", "550 No such user here", true},
		{"relay denied policy 5.7.1", "554 5.7.1 relay access denied", false},
		{"mailbox full 552", "552 5.2.2 mailbox over quota", false},
		{"generic policy reject", "550 rejected by policy", false},
		{"transient 450", "450 4.2.1 mailbox busy", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
			server.rejectRcptWith = tc.reply
			defer server.close()
			withSMTPSettings(t)

			SMTPServer = server.host
			SMTPPort = server.port
			SMTPSSLEnabled = false
			SMTPStartTLSEnabled = false
			SMTPAccount = ""
			SMTPFrom = "sender@example.com"
			SMTPToken = ""
			SystemName = "New API"

			err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
			require.Error(t, err)
			require.Equal(t, tc.permanent, IsPermanentRecipientRejection(err))
		})
	}
}

// TestSendMarketingEmailIncludesExtraHeaders guards the List-Unsubscribe
// contract: extra headers must land in the message header block, and CR/LF in a
// header must be rejected before anything reaches the wire.
func TestSendMarketingEmailIncludesExtraHeaders(t *testing.T) {
	server := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPAccount = ""
	SMTPFrom = "sender@example.com"
	SMTPToken = ""
	SystemName = "New API"

	err := SendMarketingEmail("Promo", "receiver@example.com", "<p>promo</p>",
		"List-Unsubscribe: <https://example.com/api/unsubscribe?token=abc>",
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click")
	require.NoError(t, err)

	select {
	case message := <-server.messages:
		require.Contains(t, message, "List-Unsubscribe: <https://example.com/api/unsubscribe?token=abc>")
		require.Contains(t, message, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}

	err = SendMarketingEmail("Promo", "receiver2@example.com", "<p>promo</p>", "Evil: a\r\nBcc: victim@example.com")
	require.Error(t, err)
}

func TestSendMarketingEmailUsesDedicatedSMTPWhenConfigured(t *testing.T) {
	mainServer := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer mainServer.close()
	marketingServer := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer marketingServer.close()
	withSMTPSettings(t)

	SMTPServer = mainServer.host
	SMTPPort = mainServer.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = ""
	SMTPFrom = "verify@example.com"
	SMTPToken = ""
	MarketingSMTPServer = marketingServer.host
	MarketingSMTPPort = marketingServer.port
	MarketingSMTPFrom = "news@example.com"
	SystemName = "New API"

	err := SendMarketingEmail("Promo", "receiver@example.com", "<p>promo</p>")
	require.NoError(t, err)

	select {
	case message := <-marketingServer.messages:
		require.Contains(t, message, "<p>promo</p>")
		require.Contains(t, message, "news@example.com")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for marketing SMTP DATA")
	}

	select {
	case message := <-mainServer.messages:
		t.Fatalf("marketing email unexpectedly delivered through main SMTP: %s", message)
	default:
	}
}

func TestSendMarketingEmailFallsBackToMainSMTPWhenUnconfigured(t *testing.T) {
	mainServer := newFakeSMTPServerWithSTARTTLSAdvertisement(t, false)
	defer mainServer.close()
	withSMTPSettings(t)

	SMTPServer = mainServer.host
	SMTPPort = mainServer.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = false
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = ""
	SMTPFrom = "verify@example.com"
	SMTPToken = ""
	MarketingSMTPServer = ""
	SystemName = "New API"

	err := SendMarketingEmail("Promo", "receiver@example.com", "<p>promo</p>")
	require.NoError(t, err)

	select {
	case message := <-mainServer.messages:
		require.Contains(t, message, "<p>promo</p>")
		require.Contains(t, message, "verify@example.com")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP DATA")
	}
}

func TestSendEmailExplicitStartTLSRejectsUntrustedCertificateByDefault(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	withSMTPSettings(t)

	SMTPServer = server.host
	SMTPPort = server.port
	SMTPSSLEnabled = false
	SMTPStartTLSEnabled = true
	SMTPInsecureSkipVerify = false
	SMTPForceAuthLogin = false
	SMTPAccount = "sender@example.com"
	SMTPFrom = "sender@example.com"
	SMTPToken = "secret"
	SystemName = "New API"

	err := SendEmail("Verification", "receiver@example.com", "<p>123456</p>")
	require.Error(t, err)
	require.Contains(t, fmt.Sprint(err), "certificate")
}
