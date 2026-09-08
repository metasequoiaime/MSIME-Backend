package account

import (
	"errors"
	"net/mail"
	"net/url"
	"os"
	"strings"
)

type OIDCConfig struct {
	ClientIDs []string `json:"client_ids"`
}
type WechatConfig struct {
	AppID       string `json:"app_id"`
	SecretEnv   string `json:"secret_env"`
	RedirectURI string `json:"redirect_uri"`
}
type MailConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	PasswordEnv string `json:"password_env"`
	From        string `json:"from"`
}
type SMSConfig struct {
	Region             string `json:"region"`
	AccessKeyIDEnv     string `json:"access_key_id_env"`
	AccessKeySecretEnv string `json:"access_key_secret_env"`
	SignName           string `json:"sign_name"`
	TemplateCode       string `json:"template_code"`
}
type Config struct {
	Enabled     bool         `json:"enabled"`
	DatabaseEnv string       `json:"database_env"`
	PepperEnv   string       `json:"pepper_env"`
	Google      OIDCConfig   `json:"google"`
	Apple       OIDCConfig   `json:"apple"`
	Wechat      WechatConfig `json:"wechat"`
	SMS         SMSConfig    `json:"sms"`
	Email       MailConfig   `json:"email"`
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if os.Getenv(c.DatabaseEnv) == "" || len(os.Getenv(c.PepperEnv)) < 32 {
		return errors.New("用户数据库及至少 32 字节的验证码密钥未配置")
	}
	for _, provider := range []OIDCConfig{c.Apple, c.Google} {
		if len(provider.ClientIDs) > 10 {
			return errors.New("每个提供方最多配置 10 个 Client ID")
		}
		seen := map[string]bool{}
		for _, id := range provider.ClientIDs {
			if id == "" || len(id) > 255 || strings.TrimSpace(id) != id || seen[id] {
				return errors.New("Client ID 必须非空且唯一")
			}
			seen[id] = true
		}
	}
	if c.Wechat.AppID != "" {
		u, e := url.Parse(c.Wechat.RedirectURI)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || os.Getenv(c.Wechat.SecretEnv) == "" {
			return errors.New("微信登录配置无效")
		}
	}
	if c.Email.From != "" {
		a, e := mail.ParseAddress(c.Email.From)
		if e != nil || a.Address != c.Email.From || c.Email.Host == "" || strings.ContainsAny(c.Email.Host, "/:\r\n ") || (c.Email.Port != 465 && c.Email.Port != 587) || c.Email.Username == "" || os.Getenv(c.Email.PasswordEnv) == "" {
			return errors.New("Lark SMTP 配置无效：仅支持 TLS 465 或 STARTTLS 587")
		}
	}
	if c.SMS.TemplateCode != "" && (c.SMS.Region == "" || c.SMS.SignName == "" || os.Getenv(c.SMS.AccessKeyIDEnv) == "" || os.Getenv(c.SMS.AccessKeySecretEnv) == "") {
		return errors.New("阿里云短信配置无效")
	}
	return nil
}
