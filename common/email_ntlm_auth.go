package common

import (
	"errors"
	"net/smtp"
	"strings"

	ntlmssp "github.com/Azure/go-ntlmssp"
)

type smtpAutoAuth struct {
	cfg  SMTPConfig
	mech string
}

func AutoSMTPAuth(cfg SMTPConfig) smtp.Auth {
	return &smtpAutoAuth{cfg: cfg}
}

func (a *smtpAutoAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	useLoginAuth := a.cfg.ForceAuthLogin
	if !useLoginAuth && a.cfg.shouldUseLoginAuth() {
		useLoginAuth = !(server != nil && len(server.Auth) == 1 && smtpServerSupportsAuth(server, "NTLM"))
	}
	if useLoginAuth {
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	}

	switch {
	case smtpServerSupportsAuth(server, "PLAIN"):
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.cfg.Account, a.cfg.Token, a.cfg.Server).Start(server)
	case smtpServerSupportsAuth(server, "LOGIN"):
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	case smtpServerSupportsAuth(server, "NTLM"):
		a.mech = "NTLM"
		negotiateMessage, err := ntlmssp.NewNegotiateMessage("", "")
		if err != nil {
			return "", nil, err
		}
		return "NTLM", negotiateMessage, nil
	default:
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.cfg.Account, a.cfg.Token, a.cfg.Server).Start(server)
	}
}

func (a *smtpAutoAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	switch a.mech {
	case "LOGIN":
		switch string(fromServer) {
		case "Username:":
			return []byte(a.cfg.Account), nil
		case "Password:":
			return []byte(a.cfg.Token), nil
		default:
			return nil, errors.New("unknown SMTP AUTH LOGIN challenge")
		}
	case "NTLM":
		return ntlmssp.NewAuthenticateMessage(fromServer, a.cfg.Account, a.cfg.Token, nil)
	default:
		return nil, errors.New("unexpected SMTP auth challenge")
	}
}

func smtpServerSupportsAuth(server *smtp.ServerInfo, mechanism string) bool {
	if server == nil {
		return false
	}
	for _, auth := range server.Auth {
		if strings.EqualFold(auth, mechanism) {
			return true
		}
	}
	return false
}
