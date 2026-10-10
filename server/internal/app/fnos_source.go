package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type fnosSession struct {
	socket               *websocket.Conn
	publicKey, sessionID string
}

func (m *MediaSourceManager) connectFnos(ctx context.Context, source MediaSource) (*fnosSession, error) {
	u, err := url.Parse(source.BaseURL)
	if err != nil {
		return nil, errors.New("飞牛地址无效")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/websocket"
	u.RawQuery = "type=main"
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPClient: m.client})
	if err != nil {
		return nil, errors.New("无法连接飞牛 WebSocket 接口")
	}
	conn.SetReadLimit(8 << 20)
	s := &fnosSession{socket: conn}
	response, err := s.call(ctx, "util.crypto.getRSAPub", nil, "")
	if err != nil {
		conn.CloseNow()
		return nil, err
	}
	s.publicKey = textValue(response["pub"])
	s.sessionID = textValue(response["si"])
	if s.publicKey == "" || s.sessionID == "" {
		conn.CloseNow()
		return nil, errors.New("飞牛握手响应无效")
	}
	return s, nil
}

func (s *fnosSession) read(ctx context.Context, reqID string) (map[string]any, error) {
	for i := 0; i < 100; i++ {
		_, data, err := s.socket.Read(ctx)
		if err != nil {
			return nil, errors.New("飞牛连接中断或请求超时")
		}
		var response map[string]any
		if json.Unmarshal(data, &response) != nil {
			return nil, errors.New("飞牛响应无效")
		}
		if reqID == "" || textValue(response["reqid"]) == reqID {
			return response, nil
		}
	}
	return nil, errors.New("飞牛响应未匹配请求")
}

func (s *fnosSession) call(ctx context.Context, method string, payload map[string]any, secret string) (map[string]any, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	id := fmt.Sprintf("%x", time.Now().UnixNano())
	payload["req"] = method
	payload["reqid"] = id
	data, _ := json.Marshal(payload)
	if secret != "" {
		key, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			return nil, errors.New("飞牛登录签名无效")
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(data)
		data = append([]byte(base64.StdEncoding.EncodeToString(mac.Sum(nil))), data...)
	}
	if err := s.socket.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, errors.New("飞牛请求发送失败")
	}
	return s.read(ctx, id)
}

func fnosSuccess(response map[string]any) error {
	if textValue(response["result"]) == "fail" || numberValue(response["errno"]) != 0 {
		return fmt.Errorf("飞牛操作失败（%d），请检查账号、验证器或权限", numberValue(response["errno"]))
	}
	return nil
}

func fnosEncrypt(publicKey string, key, iv []byte, payload map[string]any) ([]byte, error) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return nil, errors.New("飞牛公钥无效")
	}
	var pub *rsa.PublicKey
	if parsed, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		pub, _ = parsed.(*rsa.PublicKey)
	} else {
		pub, _ = x509.ParsePKCS1PublicKey(block.Bytes)
	}
	if pub == nil || pub.N.BitLen() < 2048 {
		return nil, errors.New("飞牛公钥不受支持")
	}
	encryptedKey, err := rsa.EncryptPKCS1v15(rand.Reader, pub, key)
	if err != nil {
		return nil, errors.New("无法加密飞牛登录")
	}
	plaintext, _ := json.Marshal(payload)
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	plaintext = append(plaintext, bytes.Repeat([]byte{byte(padding)}, padding)...)
	blockCipher, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	encrypted := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(blockCipher, iv).CryptBlocks(encrypted, plaintext)
	return json.Marshal(map[string]string{"req": "encrypted", "iv": base64.StdEncoding.EncodeToString(iv), "rsa": base64.StdEncoding.EncodeToString(encryptedKey), "aes": base64.StdEncoding.EncodeToString(encrypted)})
}

func fnosDecryptSecret(encoded string, key, iv []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return "", errors.New("飞牛返回的会话密钥无效")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)
	n := int(data[len(data)-1])
	if n < 1 || n > aes.BlockSize || n > len(data) {
		return "", errors.New("飞牛会话密钥填充无效")
	}
	for _, b := range data[len(data)-n:] {
		if int(b) != n {
			return "", errors.New("飞牛会话密钥填充无效")
		}
	}
	return base64.StdEncoding.EncodeToString(data[:len(data)-n]), nil
}

func (m *MediaSourceManager) loginFnos(ctx context.Context, source MediaSource, in NASSourceInput) (nasCredentials, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	c := nasCredentials{Username: in.Username, Password: in.Password}
	s, err := m.connectFnos(ctx, source)
	if err != nil {
		return c, err
	}
	defer s.socket.CloseNow()
	key := make([]byte, 32)
	iv := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		return c, err
	}
	if _, err = rand.Read(iv); err != nil {
		return c, err
	}
	did := mustRandomString(18)
	payload := map[string]any{"req": "user.login", "reqid": fmt.Sprintf("%x", time.Now().UnixNano()), "user": in.Username, "password": in.Password, "stay": true, "deviceType": "Server", "deviceName": "SameFrame", "did": did, "si": s.sessionID}
	send := func(payload map[string]any) (map[string]any, error) {
		encrypted, err := fnosEncrypt(s.publicKey, key, iv, payload)
		if err != nil {
			return nil, err
		}
		if err = s.socket.Write(ctx, websocket.MessageText, encrypted); err != nil {
			return nil, errors.New("飞牛登录请求失败")
		}
		return s.read(ctx, "")
	}
	response, err := send(payload)
	if err != nil {
		return c, err
	}
	if textValue(response["accessToken"]) != "" && textValue(response["token"]) == "" {
		if len(in.OTP) != 6 {
			return c, errors.New("此飞牛账号需要六位验证器验证码，请填写后重试")
		}
		response, err = send(map[string]any{"req": "user.2fa.loginVerify", "reqid": fmt.Sprintf("%x", time.Now().UnixNano()), "code": in.OTP, "isTrustedDevice": false, "accessToken": response["accessToken"], "stay": 1, "deviceName": "SameFrame", "deviceType": "Server", "did": did, "si": s.sessionID})
		if err != nil {
			return c, err
		}
	}
	if err = fnosSuccess(response); err != nil {
		return c, err
	}
	c.Token = textValue(response["token"])
	c.LongToken = textValue(response["longToken"])
	if c.Token == "" {
		return c, errors.New("飞牛没有返回登录令牌")
	}
	c.Secret, err = fnosDecryptSecret(textValue(response["secret"]), key, iv)
	if err != nil {
		return c, err
	}
	if in.WebDAVURL != "" {
		c.WebDAVURL = in.WebDAVURL
		c.WebDAVRoot = "/"
	} else {
		auth, err := m.authenticateFnos(ctx, source, c)
		if err != nil {
			return c, err
		}
		defer auth.socket.CloseNow()
		config, err := auth.call(ctx, "appcgi.share.webdav.opt", nil, c.Secret)
		if err != nil {
			return c, err
		}
		if err = fnosSuccess(config); err != nil {
			return c, err
		}
		data := objectValue(config["data"])
		if data == nil {
			data = config
		}
		if !boolValue(data["webdavEnable"]) && !boolValue(data["enable"]) && !boolValue(data["enabled"]) {
			return c, errors.New("请先在飞牛启用 WebDAV 文件服务，或填写它的地址")
		}
		for _, k := range []string{"httpsAddr", "httpsAddress", "httpAddr", "httpAddress"} {
			if value := textValue(data[k]); value != "" {
				c.WebDAVURL = value
				break
			}
		}
		if c.WebDAVURL == "" {
			base, _ := url.Parse(source.BaseURL)
			secure := base.Scheme == "https" || boolValue(data["httpsEnable"])
			keys := []string{"httpsPort", "svcPort", "port", "webdavPort"}
			scheme := "https"
			if !secure {
				keys[0] = "httpPort"
				scheme = "http"
			}
			for _, k := range keys {
				port := numberValue(data[k])
				if port > 0 && port < 65536 {
					c.WebDAVURL = scheme + "://" + net.JoinHostPort(base.Hostname(), strconv.FormatInt(port, 10))
					break
				}
			}
		}
		c.WebDAVRoot = "/"
		for _, k := range []string{"root", "path", "mountPath"} {
			if v := textValue(data[k]); v != "" {
				c.WebDAVRoot = v
				break
			}
		}
	}
	dav, err := m.validateBaseURL(ctx, c.WebDAVURL)
	if err != nil {
		return c, errors.New("飞牛 WebDAV 地址无效，请在连接窗口明确填写")
	}
	base, _ := url.Parse(source.BaseURL)
	if dav.Hostname() != base.Hostname() {
		return c, errors.New("飞牛 WebDAV 必须使用同一 NAS 主机")
	}
	c.WebDAVURL = dav.String()
	return c, nil
}

func (m *MediaSourceManager) authenticateFnos(ctx context.Context, source MediaSource, c nasCredentials) (*fnosSession, error) {
	s, err := m.connectFnos(ctx, source)
	if err != nil {
		return nil, err
	}
	response, err := s.call(ctx, "user.authToken", map[string]any{"main": true, "token": c.Token, "si": s.sessionID}, c.Secret)
	if err == nil && numberValue(response["errno"]) == 135168 && c.LongToken != "" {
		response, err = s.call(ctx, "user.tokenLogin", map[string]any{"deviceType": "Server", "deviceName": "SameFrame", "did": mustRandomString(18), "si": s.sessionID, "token": c.LongToken}, c.Secret)
	}
	if err == nil {
		err = fnosSuccess(response)
	}
	if err != nil {
		s.socket.CloseNow()
		return nil, err
	}
	return s, nil
}

func (m *MediaSourceManager) browseFnos(ctx context.Context, source MediaSource, c nasCredentials, clean string) ([]MediaFile, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s, err := m.authenticateFnos(ctx, source, c)
	if err != nil {
		return nil, err
	}
	defer s.socket.CloseNow()
	payload := map[string]any{}
	parent := strings.Trim(clean, "/")
	if parent != "" {
		payload["path"] = parent
	}
	response, err := s.call(ctx, "file.ls", payload, c.Secret)
	if err != nil {
		return nil, err
	}
	if err = fnosSuccess(response); err != nil {
		return nil, err
	}
	files := []MediaFile{}
	for _, raw := range arrayValue(response["files"]) {
		v := objectValue(raw)
		name := textValue(v["name"])
		p := path.Join("/", parent, name)
		if parent == "" && v["v"] != nil && v["uid"] != nil {
			p = fmt.Sprintf("/vol%d/%d/%s", numberValue(v["v"]), numberValue(v["uid"]), name)
		}
		files = append(files, MediaFile{Name: name, Path: p, IsDirectory: numberValue(v["dir"]) == 1, Size: numberValue(v["size"])})
	}
	return cleanNASFiles(files), nil
}

func fnosDAVPath(value, root string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) > 2 && strings.HasPrefix(parts[0], "vol") {
		_, vErr := strconv.ParseUint(strings.TrimPrefix(parts[0], "vol"), 10, 64)
		_, uErr := strconv.ParseUint(parts[1], 10, 64)
		if vErr == nil && uErr == nil {
			parts = parts[2:]
		}
	}
	return path.Join("/", root, strings.Join(parts, "/"))
}
