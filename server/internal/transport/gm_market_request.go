// 本文件核对合服清退的本机管理来源，并解析只接受规范字段的少量运维输入。
package transport

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	p "minimir/internal/protocol"
)

const gmMarketPath = "/api/maintenance/market-merge" // 该命名空间绕开开发浏览器的通用 CORS。
const gmMarketMaxBody = 2048                         // 运维命令仅携带原身份和数字版本。

// gmMarketRequestPath 标识仅 GM 能访问的整个清退命名空间，包括不受理的方法。
func gmMarketRequestPath(path string) bool {
	return path == gmMarketPath || strings.HasPrefix(path, gmMarketPath+"/")
}

// gmMarketAuthorize 同时核对实际对端、原本机监听、浏览器请求来源及原节点凭据。
func (s *Server) gmMarketAuthorize(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	ip, parseErr := netip.ParseAddr(peer)
	// Host 与传入连接绑定到同一 GM 地址，禁止浏览器来源和经过改写来源的代理。
	if err != nil || parseErr != nil || !ip.IsLoopback() || r.Host != s.Store.Cfg.GMAddress || r.URL.RawPath != "" ||
		len(r.Header.Values("Origin")) != 0 || len(r.Header.Values("Sec-Fetch-Site")) != 0 ||
		len(r.Header.Values("Sec-Fetch-Mode")) != 0 || len(r.Header.Values("Sec-Fetch-Dest")) != 0 ||
		len(r.Header.Values("Sec-Fetch-User")) != 0 || len(r.Header.Values("Forwarded")) != 0 ||
		len(r.Header.Values("X-Forwarded-For")) != 0 || len(r.Header.Values("X-Forwarded-Host")) != 0 {
		writeGMMarketError(w, http.StatusForbidden, p.CodeLineForbidden, "清退接口仅受理本机 GM 客户端")
		return false
	}
	keys := r.Header.Values("X-Dev-Node-Key")
	if s.NodeKey == "" || len(keys) != 1 || subtle.ConstantTimeCompare([]byte(keys[0]), []byte(s.NodeKey)) != 1 {
		writeGMMarketError(w, http.StatusUnauthorized, p.CodeUnauthenticated, "GM 节点身份未确认")
		return false
	}
	return true
}

// writeGMMarketError 返回短而稳定的业务错误，避免把数据库、文件路径和凭据反射给请求者。
func writeGMMarketError(w http.ResponseWriter, status int, code uint32, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	respond(w, map[string]any{"error_code": code, "message": message})
}

// gmMarketBody 核对内容类型、小包体和唯一规范键，再以强类型解析各字段。
// encoding/json 默认接受重复键和大小写变体，先校验键序列可使原请求的幂等身份没有歧义。
func gmMarketBody[T any](w http.ResponseWriter, r *http.Request, fields ...string) (T, bool) {
	var request T
	if r.URL.RawQuery != "" {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退命令参数只能放在 JSON 正文")
		return request, false
	}
	contentType := r.Header.Values("Content-Type")
	if len(contentType) != 1 || len(r.Header.Values("Content-Encoding")) != 0 {
		writeGMMarketError(w, http.StatusUnsupportedMediaType, p.CodeInvalidArgument, "清退命令只接受 JSON")
		return request, false
	}
	mediaType, params, err := mime.ParseMediaType(contentType[0])
	if err != nil || mediaType != "application/json" || len(params) > 1 || (len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeGMMarketError(w, http.StatusUnsupportedMediaType, p.CodeInvalidArgument, "清退命令只接受 UTF-8 JSON")
		return request, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, gmMarketMaxBody))
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		writeGMMarketError(w, http.StatusRequestEntityTooLarge, p.CodeInvalidArgument, "清退请求超过大小限制")
		return request, false
	}
	if err != nil || !utf8.Valid(raw) || !gmMarketObject(raw, fields) || json.Unmarshal(raw, &request) != nil {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退命令字段或类型不合法")
		return request, false
	}
	return request, true
}

// gmMarketObject 只接受预先登记的全部字段，每个字段恰好一次且不能为 null。
func gmMarketObject(raw []byte, fields []string) bool {
	allowed := make(map[string]bool, len(fields))
	for _, name := range fields {
		allowed[name] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || !allowed[name] {
			return false
		}
		delete(allowed, name)
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(value, []byte("null")) {
			return false
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') || len(allowed) != 0 {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

// gmMarketQuery 不接受额外条件、重复参数或携带请求体的只读运维请求。
func gmMarketQuery(r *http.Request, fields ...string) (url.Values, bool) {
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.URL.RawQuery) > gmMarketMaxBody {
		return nil, false
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != len(fields) {
		return nil, false
	}
	for _, field := range fields {
		if len(query[field]) != 1 || query[field][0] == "" {
			return nil, false
		}
	}
	return query, true
}

// gmMarketVersion 解析只读证明查询携带的 BSON 安全整数版本。
func gmMarketVersion(value string) (int64, bool) {
	version, err := strconv.ParseInt(value, 10, 64)
	return version, err == nil && version > 0 && strconv.FormatInt(version, 10) == value
}
