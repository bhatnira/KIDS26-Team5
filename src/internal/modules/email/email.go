// Package email provides a business-independent SMTP mailer backed by go-mail,
// with support for HTML and plain-text templates, SMTP authentication, and TLS.
package email

import (
	"crypto/rand"
	"errors"
	"fmt"
	htmltmpl "html/template"
	"io"
	"io/fs"
	"math/big"
	"net/mail"
	"path"
	"regexp"
	"strings"
	texttmpl "text/template"

	"antelope/internal/modules/log"
	"antelope/internal/modules/setting"

	gomail "github.com/wneessen/go-mail"
	"go.uber.org/zap"
)

const (
	// TLS policy config values.
	tlsPolicyMandatory = "mandatory"
	tlsPolicyNoSSLTLS  = "nossltls"
	tlsPolicyNoTLS     = "notls"
	tlsPolicyDisabled  = "disabled"

	smtpAuthLogin      = "login"
	smtpAuthCramMD5    = "crammd5"
	smtpAuthCramMD5Alt = "cram-md5"
	smtpAuthNone       = "none"
	smtpAuthNo         = "no"

	emailRegexString = "^(?:(?:(?:(?:[a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+(?:\\.([a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+)*)|(?:(?:\\x22)(?:(?:(?:(?:\\x20|\\x09)*(?:\\x0d\\x0a))?(?:\\x20|\\x09)+)?(?:(?:[\\x01-\\x08\\x0b\\x0c\\x0e-\\x1f\\x7f]|\\x21|[\\x23-\\x5b]|[\\x5d-\\x7e]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[\\x01-\\x09\\x0b\\x0c\\x0d-\\x7f]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}]))))*(?:(?:(?:\\x20|\\x09)*(?:\\x0d\\x0a))?(\\x20|\\x09)+)?(?:\\x22))))@(?:(?:(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])(?:[a-zA-Z]|\\d|-|\\.|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*(?:[a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.)+(?:(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(?:(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])(?:[a-zA-Z]|\\d|-|\\.|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*(?:[a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.?$"
)

var emailRegexp = regexp.MustCompile(emailRegexString)

// Mailer is a ready-to-use SMTP mailer. Create one via New and reuse it
// across the lifetime of the application.
//
// Note: DialAndSend opens a new TCP connection for every call. The underlying
// go-mail client does not maintain a persistent connection, so Mailer is safe
// for concurrent use but is not optimized for bulk sending.
type Mailer struct {
	cfg    setting.EmailConfig
	client *gomail.Client
}

// newMailer creates a Mailer from the application's EmailConfig.
// The returned Mailer should be kept as a long-lived singleton (e.g. stored in global.MAILER).
// Configuration errors (bad host, port, auth type, TLS policy) are returned immediately;
// connectivity problems are only surfaced when Send or SendRaw is first called.
func newMailer(cfg setting.EmailConfig) (*Mailer, error) {
	opts := []gomail.Option{
		gomail.WithPort(cfg.Port),
		gomail.WithTLSPolicy(parseTLSPolicy(cfg.TLSPolicy)),
	}

	if cfg.Username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(parseSMTPAuthType(cfg.AuthType)),
			gomail.WithUsername(cfg.Username),
			gomail.WithPassword(cfg.Password),
		)
	}

	c, err := gomail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("email: create client: %w", err)
	}

	return &Mailer{cfg: cfg, client: c}, nil
}

// InitMailer creates a Mailer from cfg and logs the outcome. It returns nil (with a
// warning) when no host is configured, and nil (with an error log) if
// construction fails. Callers should store the result and check for nil before
// use.
func InitMailer(cfg setting.EmailConfig) *Mailer {
	if cfg.Host == "" {
		log.L().Warn("email host not configured, mailer disabled")
		return nil
	}
	m, err := newMailer(cfg)
	if err != nil {
		log.L().Error("failed to initialise mailer", zap.Error(err))
		return nil
	}
	log.L().Info("mailer initialised", zap.String("host", cfg.Host), zap.Int("port", cfg.Port))
	return m
}

// formatFrom builds an RFC 5322-compliant "From" address string.
// net/mail.Address.String() quotes the display name automatically when it
// contains characters that require quoting (commas, angle brackets, etc.).
func (m *Mailer) formatFrom() string {
	return (&mail.Address{Name: m.cfg.Nickname, Address: m.cfg.From}).String()
}

// SendOptions describes a single outbound email.
type SendOptions struct {
	// To is the list of recipient addresses (required, at least one).
	To []string

	// Subject is the email subject line (required).
	Subject string

	// Templates is the filesystem that template names are resolved against
	// (typically an embedded FS). Required when a template name is set.
	Templates fs.FS

	// HTMLTemplatePath is the name, within Templates, of a html/template file
	// that renders the HTML body (e.g. "mail/user/auth/verification_code.tmpl").
	// Either HTMLTemplatePath or TextTemplatePath (or both) must be provided.
	HTMLTemplatePath string

	// TextTemplatePath is the name, within Templates, of a text/template file
	// that renders a plain-text alternative body. When both names are set the
	// message is sent as multipart/alternative (HTML primary, text fallback).
	TextTemplatePath string

	// TemplateData is passed as the dot value when executing the template(s).
	TemplateData any
}

// Send renders the configured template(s) and delivers the email via SMTP.
func (m *Mailer) Send(opts SendOptions) error {
	if len(opts.To) == 0 {
		return errors.New("email: no recipients specified")
	}
	if opts.HTMLTemplatePath == "" && opts.TextTemplatePath == "" {
		return errors.New("email: at least one of HTMLTemplatePath or TextTemplatePath must be set")
	}
	if opts.Templates == nil {
		return errors.New("email: Templates filesystem must be set")
	}

	msg := gomail.NewMsg()

	if err := msg.From(m.formatFrom()); err != nil {
		return fmt.Errorf("email: set from: %w", err)
	}

	for _, to := range opts.To {
		if err := msg.AddTo(to); err != nil {
			return fmt.Errorf("email: add recipient %q: %w", to, err)
		}
	}

	msg.Subject(opts.Subject)

	// Render HTML body.
	if opts.HTMLTemplatePath != "" {
		tmpl, err := htmltmpl.ParseFS(opts.Templates, opts.HTMLTemplatePath)
		if err != nil {
			return fmt.Errorf("email: parse html template %q: %w", opts.HTMLTemplatePath, err)
		}
		// ParseFS names the template after the file's base name.
		// Look it up explicitly so that templates using {{define}} blocks
		// are resolved correctly regardless of how many files are parsed.
		name := path.Base(opts.HTMLTemplatePath)
		t := tmpl.Lookup(name)
		if t == nil {
			return fmt.Errorf("email: html template %q not found in parsed set", name)
		}
		if err = msg.SetBodyHTMLTemplate(t, opts.TemplateData); err != nil {
			return fmt.Errorf("email: set html body: %w", err)
		}
	}

	// Render plain-text alternative (or sole body when no HTML template given).
	if opts.TextTemplatePath != "" {
		ttmpl, err := texttmpl.ParseFS(opts.Templates, opts.TextTemplatePath)
		if err != nil {
			return fmt.Errorf("email: parse text template %q: %w", opts.TextTemplatePath, err)
		}
		// Same explicit lookup as for the HTML template above.
		name := path.Base(opts.TextTemplatePath)
		t := ttmpl.Lookup(name)
		if t == nil {
			return fmt.Errorf("email: text template %q not found in parsed set", name)
		}
		if opts.HTMLTemplatePath != "" {
			// HTML is the primary part; add text as a fallback alternative.
			if err = msg.AddAlternativeTextTemplate(t, opts.TemplateData); err != nil {
				return fmt.Errorf("email: add text alternative: %w", err)
			}
		} else {
			if err = msg.SetBodyTextTemplate(t, opts.TemplateData); err != nil {
				return fmt.Errorf("email: set text body: %w", err)
			}
		}
	}

	if err := m.client.DialAndSend(msg); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

// SendRaw delivers an email with a pre-rendered HTML body string. This is a
// convenience helper for callers that build the body themselves rather than
// using a template file.
func (m *Mailer) SendRaw(to []string, subject, htmlBody string) error {
	if len(to) == 0 {
		return errors.New("email: no recipients specified")
	}

	msg := gomail.NewMsg()

	if err := msg.From(m.formatFrom()); err != nil {
		return fmt.Errorf("email: set from: %w", err)
	}

	for _, addr := range to {
		if err := msg.AddTo(addr); err != nil {
			return fmt.Errorf("email: add recipient %q: %w", addr, err)
		}
	}

	msg.Subject(subject)

	msg.SetBodyWriter(gomail.TypeTextHTML, func(w io.Writer) (int64, error) {
		n, err := fmt.Fprint(w, htmlBody)
		return int64(n), err
	})

	if err := m.client.DialAndSend(msg); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}

// VerifyEmailFormat reports whether the given string is a well-formed email address.
func VerifyEmailFormat(email string) bool {
	return emailRegexp.MatchString(email)
}

// RandomCode returns a cryptographically secure random numeric string of length n.
// It uses crypto/rand so the result is suitable for security-sensitive contexts
// such as email verification codes and one-time passwords.
func RandomCode(n int) string {
	const digits = "0123456789"
	code := make([]byte, n)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			// crypto/rand failure is an unrecoverable system error.
			panic(fmt.Sprintf("email: RandomCode: crypto/rand failed: %v", err))
		}
		code[i] = digits[num.Int64()]
	}
	return string(code)
}

// parseTLSPolicy converts the config string to a go-mail TLSPolicy constant.
// Recognised values and their meanings:
//
//	"mandatory"  – TLS is required; connection fails if the server does not support it.
//	"nossltls"   – Disable TLS entirely (plain-text SMTP).
//	"notls"      – Alias for "nossltls".
//	"disabled"   – Alias for "nossltls" (preferred; unambiguous).
//
// Any other value (including empty string) falls back to TLSOpportunistic:
// TLS is attempted first and the connection downgrades to plain-text if unavailable.
func parseTLSPolicy(policy string) gomail.TLSPolicy {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case tlsPolicyMandatory:
		return gomail.TLSMandatory
	case tlsPolicyNoSSLTLS, tlsPolicyNoTLS, tlsPolicyDisabled:
		return gomail.NoTLS
	default:
		// Opportunistic: try TLS first, fall back to plain if TLS fails.
		return gomail.TLSOpportunistic
	}
}

// parseSMTPAuthType converts the config string to a go-mail SMTPAuthType.
// Defaults to SMTPAuthPlain when the value is unrecognized or empty.
func parseSMTPAuthType(authType string) gomail.SMTPAuthType {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case smtpAuthLogin:
		return gomail.SMTPAuthLogin
	case smtpAuthCramMD5, smtpAuthCramMD5Alt:
		return gomail.SMTPAuthCramMD5
	case smtpAuthNone, smtpAuthNo:
		return gomail.SMTPAuthNoAuth
	default:
		return gomail.SMTPAuthPlain
	}
}
