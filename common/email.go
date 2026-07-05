package common

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"slices"
	"strings"
	"time"
)

// smtpDialTimeout bounds the TCP/TLS connect to the SMTP server.
const smtpDialTimeout = 30 * time.Second

// smtpSessionTimeout bounds one full SMTP session (connect through QUIT) via a
// connection deadline, so a stalled server can never hang a caller forever. The
// bulk-email recovery sweep relies on every send finishing (or failing) within
// this bound to tell live campaigns apart from orphaned ones.
const smtpSessionTimeout = 2 * time.Minute

// SMTPConfig carries one complete set of SMTP settings. Transactional mail
// (verification codes, notifications) uses the main settings; marketing/bulk
// mail may use a dedicated set so providers like Aliyun DirectMail can keep the
// two sender reputations apart.
type SMTPConfig struct {
	Server             string
	Port               int
	SSLEnabled         bool
	StartTLSEnabled    bool
	InsecureSkipVerify bool
	ForceAuthLogin     bool
	Account            string
	From               string
	Token              string
}

func mainSMTPConfig() SMTPConfig {
	return SMTPConfig{
		Server:             SMTPServer,
		Port:               SMTPPort,
		SSLEnabled:         SMTPSSLEnabled,
		StartTLSEnabled:    SMTPStartTLSEnabled,
		InsecureSkipVerify: SMTPInsecureSkipVerify,
		ForceAuthLogin:     SMTPForceAuthLogin,
		Account:            SMTPAccount,
		From:               SMTPFrom,
		Token:              SMTPToken,
	}
}

// marketingSMTPConfig returns the dedicated bulk-email SMTP settings when the
// admin configured them, falling back to the main SMTP settings otherwise. A
// non-empty MarketingSMTPServer is what marks the dedicated set as configured;
// once set, the marketing values are used as-is (no per-field mixing with the
// main settings, which would pair credentials with the wrong server).
func marketingSMTPConfig() SMTPConfig {
	if MarketingSMTPServer == "" {
		return mainSMTPConfig()
	}
	return SMTPConfig{
		Server:             MarketingSMTPServer,
		Port:               MarketingSMTPPort,
		SSLEnabled:         MarketingSMTPSSLEnabled,
		StartTLSEnabled:    MarketingSMTPStartTLSEnabled,
		InsecureSkipVerify: MarketingSMTPInsecureSkipVerify,
		ForceAuthLogin:     MarketingSMTPForceAuthLogin,
		Account:            MarketingSMTPAccount,
		From:               MarketingSMTPFrom,
		Token:              MarketingSMTPToken,
	}
}

func generateMessageID(from string) (string, error) {
	split := strings.Split(from, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := split[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func (cfg SMTPConfig) shouldUseLoginAuth() bool {
	if cfg.ForceAuthLogin {
		return true
	}
	return isOutlookServer(cfg.Account) || slices.Contains(EmailLoginAuthServerList, cfg.Server)
}

func (cfg SMTPConfig) shouldAuthenticate() bool {
	return cfg.Account != "" && cfg.Token != ""
}

func (cfg SMTPConfig) tlsConfig() *tls.Config {
	return &tls.Config{
		ServerName:         cfg.Server,
		InsecureSkipVerify: cfg.InsecureSkipVerify, // #nosec G402 -- admin-controlled SMTP compatibility option.
	}
}

func newSMTPClient(cfg SMTPConfig, addr string) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	if cfg.SSLEnabled || (cfg.Port == 465 && !cfg.StartTLSEnabled) {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, cfg.tlsConfig())
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(time.Now().Add(smtpSessionTimeout))
		client, err := smtp.NewClient(conn, cfg.Server)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return client, nil
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	// The deadline set on the raw conn keeps applying after a STARTTLS upgrade,
	// bounding the whole session.
	_ = conn.SetDeadline(time.Now().Add(smtpSessionTimeout))
	client, err := smtp.NewClient(conn, cfg.Server)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if cfg.StartTLSEnabled {
		startTLSSupported, _ := client.Extension("STARTTLS")
		if !startTLSSupported {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(cfg.tlsConfig()); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return client, nil
}

// RcptRejectionError marks an SMTP failure that happened at the RCPT TO step —
// the one step whose failure is attributable to the specific recipient address
// (invalid mailbox, provider invalid-address library, recipient-side block).
// MAIL FROM or DATA failures are sender/content problems and are deliberately
// NOT wrapped: suppressing a recipient for those would be wrong.
type RcptRejectionError struct {
	Code int // SMTP reply code, 0 when the failure was not an SMTP reply
	Err  error
}

func (e *RcptRejectionError) Error() string { return e.Err.Error() }
func (e *RcptRejectionError) Unwrap() error { return e.Err }

// recipientInvalidPhrases are RCPT reply fragments that specifically say the
// destination mailbox does not exist, as opposed to policy/relay/quota
// failures that merely happened at the RCPT step.
var recipientInvalidPhrases = []string{
	"user unknown",
	"unknown user",
	"no such user",
	"user not found",
	"does not exist",
	"invalid recipient",
	"unknown recipient",
	"recipient not found",
	"invalid mailbox",
	"mailbox not found",
	"invalid address",
	"nonexistent",
}

// IsPermanentRecipientRejection reports whether err is a permanent (5xx) SMTP
// rejection that identifies the RECIPIENT ADDRESS as invalid — the only signal
// that justifies adding the address to the suppression list. It requires the
// enhanced status class 5.1.x (bad destination mailbox) or an explicit
// "no such user"-style phrase, NOT merely any 5xx at the RCPT step: a
// misconfigured relay answers every RCPT with 5.7.x "relaying denied" and a
// full mailbox answers 552/5.2.2, and treating those as hard bounces would
// permanently poison the suppression list with perfectly good addresses.
// Unrecognized rejections are simply not learned from — the safe direction.
func IsPermanentRecipientRejection(err error) bool {
	var rcptErr *RcptRejectionError
	if !errors.As(err, &rcptErr) || rcptErr.Code < 500 || rcptErr.Code >= 600 {
		return false
	}
	msg := strings.ToLower(rcptErr.Err.Error())
	// RFC 3463 enhanced status 5.1.x = bad destination mailbox address.
	if strings.Contains(msg, "5.1.") {
		return true
	}
	for _, phrase := range recipientInvalidPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

// SendEmail sends transactional mail (verification codes, password resets,
// per-user notifications) through the main SMTP settings.
func SendEmail(subject string, receiver string, content string) error {
	return sendEmailWithConfig(mainSMTPConfig(), subject, receiver, content, nil)
}

// SendMarketingEmail sends marketing/bulk mail through the dedicated marketing
// SMTP settings when configured, otherwise through the main SMTP settings.
// extraHeaders are raw "Name: value" header lines (e.g. List-Unsubscribe)
// inserted verbatim into the message header block.
func SendMarketingEmail(subject string, receiver string, content string, extraHeaders ...string) error {
	return sendEmailWithConfig(marketingSMTPConfig(), subject, receiver, content, extraHeaders)
}

func sendEmailWithConfig(cfg SMTPConfig, subject string, receiver string, content string, extraHeaders []string) error {
	if cfg.From == "" { // for compatibility
		cfg.From = cfg.Account
	}
	// Defense-in-depth against SMTP header injection: the receiver and From values
	// are written verbatim into raw mail headers below, so reject any CR/LF before
	// they can inject additional headers (e.g. a smuggled Bcc).
	if strings.ContainsAny(receiver, "\r\n") || strings.ContainsAny(cfg.From, "\r\n") || strings.ContainsAny(SystemName, "\r\n") {
		return fmt.Errorf("invalid email header value")
	}
	id, err2 := generateMessageID(cfg.From)
	if err2 != nil {
		return err2
	}
	if cfg.Server == "" && cfg.Account == "" {
		return fmt.Errorf("SMTP 服务器未配置")
	}
	// Extra headers are always constructed by our own code, but reject CR/LF
	// anyway so a future caller can never smuggle additional headers through.
	extraHeaderBlock := ""
	for _, h := range extraHeaders {
		if strings.ContainsAny(h, "\r\n") || !strings.Contains(h, ":") {
			return fmt.Errorf("invalid extra email header")
		}
		extraHeaderBlock += h + "\r\n"
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s <%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"%s"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, SystemName, cfg.From, encodedSubject, time.Now().Format(time.RFC1123Z), id, extraHeaderBlock, content))
	auth := AutoSMTPAuth(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.Server, cfg.Port)
	to := strings.Split(receiver, ";")
	var err error
	client, err := newSMTPClient(cfg, addr)
	if err != nil {
		return err
	}
	defer client.Close()
	if cfg.shouldAuthenticate() {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(cfg.From); err != nil {
		return err
	}
	for _, receiver := range to {
		if err = client.Rcpt(receiver); err != nil {
			code := 0
			var tpErr *textproto.Error
			if errors.As(err, &tpErr) {
				code = tpErr.Code
			}
			return &RcptRejectionError{Code: code, Err: err}
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(mail)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	// The message was accepted by the server once DATA closed successfully. A
	// failure during QUIT (or the session deadline firing here) must not be
	// reported as a send failure — callers would wrongly count a delivered email
	// as failed (and a user might be told their verification mail never went out).
	if err = client.Quit(); err != nil {
		SysError(fmt.Sprintf("email to %s delivered but QUIT failed: %v", receiver, err))
	}
	return nil
}
