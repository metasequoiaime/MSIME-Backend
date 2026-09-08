package account

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/dysmsapi"
)

type delivery struct {
	config      Config
	smtpTLS     *tls.Config
	smtpAddress string
}
type contextTransport struct{ ctx context.Context }

func (t contextTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(r.Clone(t.ctx))
}
func (d delivery) Send(ctx context.Context, channel, target, code string) error {
	var e error
	switch channel {
	case "phone":
		e = d.sms(ctx, target, code)
	case "email":
		e = d.email(ctx, target, code)
	default:
		e = errors.New("渠道未配置")
	}
	// SDK/SMTP 错误可能包含手机号、验证码或凭据，不向调用者透传。
	if e != nil {
		return errors.New("验证码发送失败")
	}
	return nil
}
func (d delivery) sms(ctx context.Context, target, code string) error {
	c := d.config.SMS
	cfg := sdk.NewConfig().WithScheme("HTTPS").WithTimeout(10 * time.Second).WithAutoRetry(false)
	cfg.Transport = contextTransport{ctx}
	client, e := dysmsapi.NewClientWithOptions(c.Region, cfg, &credentials.AccessKeyCredential{AccessKeyId: os.Getenv(c.AccessKeyIDEnv), AccessKeySecret: os.Getenv(c.AccessKeySecretEnv)})
	if e != nil {
		return e
	}
	r := dysmsapi.CreateSendSmsRequest()
	r.SetScheme("HTTPS")
	r.SetDomain("dysmsapi.aliyuncs.com")
	r.PhoneNumbers = strings.TrimPrefix(target, "+")
	r.SignName = c.SignName
	r.TemplateCode = c.TemplateCode
	params, _ := json.Marshal(map[string]string{"code": code})
	r.TemplateParam = string(params)
	result, e := client.SendSms(r)
	if e != nil {
		return e
	}
	if result.Code != "OK" {
		return errors.New("短信供应商拒绝")
	}
	return nil
}
func (d delivery) email(ctx context.Context, target, code string) error {
	c := d.config.Email
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	if d.smtpAddress != "" {
		address = d.smtpAddress
	}
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if d.smtpTLS != nil {
		tlsConfig = d.smtpTLS.Clone()
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var conn net.Conn
	var e error
	if c.Port == 465 {
		conn, e = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, e = dialer.DialContext(ctx, "tcp", address)
	}
	if e != nil {
		return e
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if v, ok := ctx.Deadline(); ok && v.Before(deadline) {
		deadline = v
	}
	conn.SetDeadline(deadline)
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return e
	}
	defer client.Close()
	if c.Port == 587 {
		if e = client.StartTLS(tlsConfig); e != nil {
			return e
		}
	}
	if e = client.Auth(smtp.PlainAuth("", c.Username, os.Getenv(c.PasswordEnv), c.Host)); e != nil {
		return e
	}
	if e = client.Mail(c.From); e != nil {
		return e
	}
	if e = client.Rcpt(target); e != nil {
		return e
	}
	body := "你的验证码是 " + code + "，5 分钟内有效。请勿向他人透露。"
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n", c.From, target, mime.QEncoding.Encode("UTF-8", "水杉输入法登录验证码"), base64.StdEncoding.EncodeToString([]byte(body)))
	writer, e := client.Data()
	if e != nil {
		return e
	}
	if _, e = writer.Write([]byte(message)); e != nil {
		writer.Close()
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	// DATA 已确认即视为成功；QUIT 失败不能触发重复投递。
	_ = client.Quit()
	return nil
}
