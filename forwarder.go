package main

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/smtp"
	"strings"
	"time"
)

type Email struct {
	From     string
	To       []string
	Subject  string
	Body     string
	SMTPHost string
	SMTPPort string
	Username string
	Password string
}

func generateMessageID(domain string) string {
	timestamp := time.Now().UnixNano()
	randomPart := rand.Int()

	return fmt.Sprintf("<%d.%d@%s>", timestamp, randomPart, domain)
}

func SendEmail(email Email) error {

	headers := make(map[string]string)
	headers["From"] = email.From
	headers["To"] = strings.Join(email.To, ",")
	headers["Subject"] = email.Subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"UTF-8\""
	headers["Date"] = time.Now().Format(time.RFC1123Z)
	headers["Message-ID"] = generateMessageID(email.SMTPHost)

	if singleRecipientOnly && len(email.To) > 1 {
		email.To = email.To[:1]
		headers["To"] = email.To[0]
	}

	var message strings.Builder
	for k, v := range headers {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n" + email.Body)

	serverAddress := email.SMTPHost + ":" + email.SMTPPort
	conn, err := smtp.Dial(serverAddress)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	defer conn.Close()

	if ok, _ := conn.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		if err = conn.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	auth := smtp.PlainAuth("", email.Username, email.Password, email.SMTPHost)
	if err = conn.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate with server: %v", err)
	}

	if err = conn.Mail(email.From); err != nil {
		return fmt.Errorf("failed to set sender: %v", err)
	}

	for _, addr := range email.To {
		if err = conn.Rcpt(addr); err != nil {
			return fmt.Errorf("failed to set recipient: %v", err)
		}
	}

	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("failed to get data: %v", err)
	}
	_, err = w.Write([]byte(message.String()))
	if err != nil {
		return fmt.Errorf("failed to write data: %v", err)
	}
	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close email writer: %v", err)
	}

	return nil
}
