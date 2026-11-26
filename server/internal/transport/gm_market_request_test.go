// 本文件验证清退接口独立的管理请求边界及数字、对象和查询参数严格解析。
package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

func TestGMMarketAuthorization(t *testing.T) {
	const host = "127.0.0.1:19091"
	const key = "本用例使用的单独节点凭据"
	server := &Server{Mode: "gm", NodeKey: key, Store: &persistence.Store{Cfg: devconfig.Config{GMAddress: host}}}
	for _, tc := range []struct {
		name string
		edit func(*http.Request)
		want int
	}{
		{"当前原管理客户端", nil, http.StatusOK},
		{"缺少认证", func(r *http.Request) { r.Header.Del("X-Dev-Node-Key") }, http.StatusUnauthorized},
		{"重复认证", func(r *http.Request) { r.Header.Add("X-Dev-Node-Key", key) }, http.StatusUnauthorized},
		{"真实远程对端", func(r *http.Request) { r.RemoteAddr = "192.0.2.2:18001" }, http.StatusForbidden},
		{"本机来源浏览器请求", func(r *http.Request) { r.Header.Set("Origin", "http://localhost:18001") }, http.StatusForbidden},
		{"空来源字段", func(r *http.Request) { r.Header.Set("Origin", "") }, http.StatusForbidden},
		{"浏览器同源请求", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") }, http.StatusForbidden},
		{"域名重新绑定", func(r *http.Request) { r.Host = "developer.example:19091" }, http.StatusForbidden},
		{"错误回环端口", func(r *http.Request) { r.Host = "127.0.0.1:19092" }, http.StatusForbidden},
		{"转发来源", func(r *http.Request) { r.Header.Set("Forwarded", "for=127.0.0.1") }, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+host+gmMarketPath+"/step", nil)
			r.RemoteAddr = "127.0.0.1:19092"
			r.Header.Set("X-Dev-Node-Key", key)
			if tc.edit != nil {
				tc.edit(r)
			}
			w := httptest.NewRecorder()
			ok := server.gmMarketAuthorize(w, r)
			if w.Code != tc.want || ok != (tc.want == http.StatusOK) || w.Header().Get("Access-Control-Allow-Origin") != "" ||
				w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("GM 边界结果错误：状态 %d 许可 %t 头 %v", w.Code, ok, w.Header())
			}
		})
	}
}

func TestGMMarketStrictInput(t *testing.T) {
	type input struct {
		PlanID  string `json:"plan_id"`
		Version int64  `json:"expected_version"`
	}
	for _, tc := range []struct {
		name, body, media string
		want              int
	}{
		{"有效精确整数", `{"plan_id":"scope","expected_version":9007199254740993}`, "application/json; charset=utf-8", 200},
		{"重复版本", `{"plan_id":"scope","expected_version":1,"expected_version":2}`, "application/json", 400},
		{"大小写变体", `{"Plan_ID":"scope","expected_version":1}`, "application/json", 400},
		{"多余字段", `{"plan_id":"scope","expected_version":1,"override":true}`, "application/json", 400},
		{"缺失字段", `{"plan_id":"scope"}`, "application/json", 400},
		{"空字段", `{"plan_id":null,"expected_version":1}`, "application/json", 400},
		{"整数字符串", `{"plan_id":"scope","expected_version":"1"}`, "application/json", 400},
		{"小数", `{"plan_id":"scope","expected_version":1.0}`, "application/json", 400},
		{"数值溢出", `{"plan_id":"scope","expected_version":9223372036854775808}`, "application/json", 400},
		{"第二个对象", `{"plan_id":"scope","expected_version":1}{}`, "application/json", 400},
		{"超长", strings.Repeat(" ", gmMarketMaxBody+1), "application/json", 413},
		{"浏览器表单", `plan_id=scope&expected_version=1`, "application/x-www-form-urlencoded", 415},
		{"错误编码", `{"plan_id":"scope","expected_version":1}`, "application/json; charset=iso-8859-1", 415},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, gmMarketPath+"/step", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			result, ok := gmMarketBody[input](w, r, "plan_id", "expected_version")
			if w.Code != tc.want || ok != (tc.want == 200) || ok && result.Version != 9007199254740993 {
				t.Fatalf("清退请求校验不同于约定：状态 %d 解析 %t 结果 %+v", w.Code, ok, result)
			}
		})
	}
	for _, raw := range []string{"plan_id=x&plan_id=y", "plan_id=x&extra=1", "plan_id=", "plan_id=%ZZ"} {
		if _, ok := gmMarketQuery(httptest.NewRequest(http.MethodGet, gmMarketPath+"/status?"+raw, nil), "plan_id"); ok {
			t.Fatal("接受了含糊或无效的只读参数", raw)
		}
	}
	for _, raw := range []string{"+2", "02", "2.0", "0", "9223372036854775808"} {
		if _, ok := gmMarketVersion(raw); ok {
			t.Fatal("接受了非规范版本", raw)
		}
	}
}
