package mail

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/ernierasta/zorix/shared"
	log "github.com/sirupsen/logrus"
)

// Mail implements Notifier interface.
type Mail struct{}

// Send sends mail via smtp.
// Supports multiple recepients, TLS (port 465)/StartTLS(ports 25,587, any other).
// Mail should always valid (correctly encoded subject and body).
func (m *Mail) Send(c shared.CheckConfig, n shared.NotifConfig) error {
	log.WithFields(log.Fields{"subj": n.Subject, "body": n.Text, "params:": c.Params}).Debug("mail.Send: New mail.")
	if (n.User != "" && n.Pass == "") ||
		(n.Pass != "" && n.User == "") ||
		n.Server == "" {
		pass := ""
		if len(n.Pass) > 3 {
			pass = n.Pass[0:3] + "..."
		} else {
			pass = n.Pass // if someone has 4 leter pass, it deserves to be logged ;-)
		}
		return fmt.Errorf("mail.Send: one of auth params is empty(SENDING ABORTED), u: %q p:%q s: %q", n.User, pass, n.Server)
	}
	auth := smtp.PlainAuth("", n.User, n.Pass, n.Server)

	recipients := strings.Join(n.To, ", ")

	header := make(map[string]string)
	header["From"] = n.From
	header["To"] = recipients
	header["Date"] = c.Timestamp.Format(time.RFC1123Z)
	header["Subject"] = encodeRFC2047(n.Subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(n.Text))
	err := sendMail(n.Server, n.Port, auth, n.IgnoreCert, n.From, n.To, []byte(message))

	p := "" // just for logging
	if len(n.Pass) >= 4 {
		p = n.Pass[0:3]
	} else {
		p = n.Pass
	}

	maillog := log.WithFields(log.Fields{
		"user":       n.User,
		"pass":       p,
		"server":     n.Server,
		"port":       n.Port,
		"ignorecert": n.IgnoreCert,
		"to":         recipients,
		"Subject":    header["Subject"]})

	if err != nil {
		maillog.Debug("error sending mail, err:", err)
		return fmt.Errorf("mail.Send: error sending mail, err: %v", err)
	}

	maillog.Debug("mail sent")

	return nil
}

func encodeRFC2047(s string) string {
	// use mail's rfc2047 to encode any string
	addr := mail.Address{s, ""}
	return strings.Trim(addr.String(), " <@>")
}

// sendMail connects to the server at addr, switches to TLS if
// possible, authenticates with the optional mechanism a if possible,
// and then sends an email from address from, to addresses to, with
// message msg.
// The addr must include a port, as in "mail.example.com:smtp".
//
// The addresses in the to parameter are the SMTP RCPT addresses.
//
// The msg parameter should be an RFC 822-style email with headers
// first, a blank line, and then the message body. The lines of msg
// should be CRLF terminated. The msg headers should usually include
// fields such as "From", "To", "Subject", and "Cc".  Sending "Bcc"
// messages is accomplished by including an email address in the to
// parameter but not including it in the msg headers.
//
// The SendMail function and the net/smtp package are low-level
// mechanisms and provide no support for DKIM signing, MIME
// attachments (see the mime/multipart package), or other mail
// functionality. Higher-level packages exist outside of the standard
// library.
//
// sendMail ripped from net/smtp package, added ability to send mails
// via TLS (port: 465).
//
// Changes for zoriX:
//  - fixed potential MITM atack
//  - added option to ignore certificate
//  - fixed stuck connection if server has port closed (added timeout)
func sendMail(host string, port int, a smtp.Auth, ignoreCert bool, from string, to []string, msg []byte) error {
	if err := validateLine(from); err != nil {
		return err
	}

	if len(to) == 0 {
		return fmt.Errorf("smtp: no recepients given")
	}
	for _, recp := range to {
		if err := validateLine(recp); err != nil {
			return err
		}
	}

	hostPort := fmt.Sprintf("%s:%d", host, port)

	c := &smtp.Client{}

	// raw network connection with timeout
	netConn, err := net.DialTimeout("tcp", hostPort, 10*time.Second)
	if err != nil {
		return err
	}

	if port == 465 {
		tlsconfig := &tls.Config{
			InsecureSkipVerify: ignoreCert,
			ServerName:         host,
		}
		conn := tls.Client(netConn, tlsconfig)
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
	} else { // using submission
		var err error
		c, err = smtp.NewClient(netConn, host)
		if err != nil {
			return err
		}
	}
	defer c.Close()
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	if err = c.Hello(hostname); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		config := &tls.Config{
			InsecureSkipVerify: ignoreCert,
			ServerName:         host,
		}
		if err = c.StartTLS(config); err != nil {
			return err
		}
	}
	if a != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp: server doesn't support AUTH")
		}
		if err = c.Auth(a); err != nil {
			return err
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

func check(err error) {
	if err != nil {
		log.Println("error sending mail, err: ", err)
	}
}

// validateLine checks to see if a line has CR or LF as per RFC 5321
func validateLine(line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return fmt.Errorf("smtp: A line must not contain CR or LF")
	}
	return nil
}
