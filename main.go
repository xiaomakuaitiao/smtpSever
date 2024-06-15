package main

import (
	"crypto/tls"
	"fmt"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"io"
	"io/ioutil"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"
)

var db *DB

var singleRecipientOnly bool //控制是否只发送一封邮件

// Backend 实现 smtp.Backend 接口
type Backend struct{}

// AnonymousLogin 实现 smtp.Backend 接口的 AnonymousLogin 方法
func (bkd *Backend) AnonymousLogin(state *smtp.Conn) (smtp.Session, error) {
	// 不允许匿名登录
	return nil, fmt.Errorf("Anonymous login not allowed")
}

// NewSession 实现 smtp.Backend 接口的 NewSession 方法
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{}, nil
}

// Session 实现 smtp.Session 接口
type Session struct {
	username string
	password string
	userData *UserData
	to       []string
	form     string
	subject  string
}

// AuthMechanisms 返回支持的身份验证机制
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Login, sasl.Plain}
}

// extractDomain 从邮件地址中提取域名
func extractDomain(from string) (string, error) {
	address, err := mail.ParseAddress(from)
	if err != nil {
		return "", err
	}
	parts := strings.Split(address.Address, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("Invalid email address")
	}
	return parts[1], nil
}

// 检查用户名和密码
func checkCredentials(username, password string) (*UserData, error) {
	userData, err := db.GetUserAndDomainData(username, password)
	if err != nil {
		return nil, err
	}

	err = db.CheckSendLimits(userData)
	if err != nil {
		return nil, err
	}

	return userData, nil
}

// Auth 实现 smtp.Session 接口的 Auth 方法
func (s *Session) Auth(mech string) (sasl.Server, error) {
	if mech == sasl.Login {
		return sasl.NewLoginServer(func(username, password string) error {
			userData, err := checkCredentials(username, password)
			if err != nil {
				return err
			}
			s.username = username
			s.password = password
			s.userData = userData
			return nil
		}), nil
	}
	if mech == sasl.Plain {
		return sasl.NewPlainServer(func(identity, username, password string) error {
			userData, err := checkCredentials(username, password)
			if err != nil {
				return err
			}
			s.username = username
			s.password = password
			s.userData = userData
			return nil
		}), nil
	}
	return nil, fmt.Errorf("unsupported mechanism")
}

// Mail 实现 smtp.Session 接口的 Mail 方法
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Println("Mail from:", from)
	// 可以在此处验证发件人地址是否允许
	domain, err := extractDomain(from)
	if err != nil {
		return err
	}
	validDomain := false
	for _, d := range s.userData.Domains {
		if d == domain {
			validDomain = true
			break
		}
	}
	if !validDomain {
		return fmt.Errorf("invalid domain")

	}
	s.form = from
	return nil
}

// Rcpt 实现 smtp.Session 接口的 Rcpt 方法
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Println("Rcpt to:", to)
	s.to = append(s.to, to)
	// 可以在此处验证收件人地址是否有效
	return nil
}

// Data 实现 smtp.Session 接口的 Data 方法
func (s *Session) Data(r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	message, err := mail.ReadMessage(strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	s.subject = message.Header.Get("Subject")
	fromHeader := message.Header.Get("From")
	from, err := mail.ParseAddress(fromHeader)
	if err != nil {
		return err
	}
	s.form = from.String()

	// 获取邮件正文
	body, err := ioutil.ReadAll(message.Body)
	if err != nil {
		return err
	}

	email := Email{
		From:     s.form,
		To:       s.to,
		Subject:  s.subject,
		Body:     string(body),
		SMTPHost: s.userData.ServerHost,
		SMTPPort: s.userData.ServerPort,
		Username: s.userData.ServerUsername,
		Password: s.userData.ServerPassword,
	}
	log.Println("Data:", email)

	if err = SendEmail(email); err != nil {
		return fmt.Errorf("failed to forward email: %v", err)
	}

	if err = db.UpdateSendLimits(s.userData.UserPlanID, len(s.to)); err != nil {
		return fmt.Errorf("failed to update send limits: %v", err)
	}

	return nil
}

// Reset 实现 smtp.Session 接口的 Reset 方法
func (s *Session) Reset() {
	// 重置会话状态
	log.Println("Session reset")
	s.username = ""
	s.password = ""
	s.userData = nil
	s.form = ""
	s.to = nil
}

// Logout 实现 smtp.Session 接口的 Logout 方法
func (s *Session) Logout() error {
	// 结束会话，清理资源
	log.Println("Session logout")
	return nil
}

// loggerWrapper 包装器类型，实现 io.Writer 接口
type loggerWrapper struct {
	logger *log.Logger
}

// Write 实现 io.Writer 接口的 Write 方法
func (lw *loggerWrapper) Write(p []byte) (n int, err error) {
	lw.logger.Print(string(p))
	return len(p), nil
}

func main() {
	// 加载配置
	config, err := LoadConfig("config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	singleRecipientOnly = config.Settings.SingleRecipientOnly

	// 初始化数据库
	db, err = GetDBInstance(config)
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}

	defer db.Close()

	// 创建SMTP服务器
	be := &Backend{}
	s := smtp.NewServer(be)

	s.Addr = config.Settings.Addr
	s.Domain = config.Settings.Domain
	s.ReadTimeout = time.Duration(config.Settings.ReadTimeout) * time.Second
	s.WriteTimeout = time.Duration(config.Settings.WriteTimeout) * time.Second
	s.MaxMessageBytes = config.Settings.MaxMessageBytes
	s.MaxRecipients = config.Settings.MaxRecipients
	s.AllowInsecureAuth = true

	// 加载TLS证书
	cert, err := tls.LoadX509KeyPair(config.Settings.CertPath, config.Settings.KeyPath)
	if err != nil {
		log.Fatal(err)
	}
	s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}

	// 启动SMTP服务器
	log.Println("Starting SMTP server on :587")
	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// 启动TLS SMTP服务器
	sTLS := smtp.NewServer(be)
	sTLS.Addr = config.Settings.TLSAddr
	sTLS.Domain = config.Settings.Domain
	sTLS.ReadTimeout = time.Duration(config.Settings.ReadTimeout) * time.Second
	sTLS.WriteTimeout = time.Duration(config.Settings.WriteTimeout) * time.Second
	sTLS.MaxMessageBytes = config.Settings.MaxMessageBytes
	sTLS.MaxRecipients = config.Settings.MaxRecipients
	sTLS.AllowInsecureAuth = true
	sTLS.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// 调整代码：使用 loggerWrapper 启用详细的日志记录
	sTLS.Debug = &loggerWrapper{logger: log.New(os.Stderr, "smtp/server ", log.LstdFlags)}

	log.Println("Starting TLS SMTP server on", config.Settings.TLSAddr)
	if err := sTLS.ListenAndServeTLS(); err != nil {
		log.Fatal(err)
	}
}
