// 本文件核对各平台的本任务专用 MongoDB 端点并拒绝外部地址。

package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDevelopmentMongoPlatformEndpoint 核验Windows必须显式使用新副本集，不能接入旧Lima转发或系统Mongo，用诊断「未选择当前平台固定端点」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDevelopmentMongoPlatformEndpoint(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MongoURI != DevelopmentMongoURI() {
		t.Fatal("未选择当前平台固定端点")
	}
	if cfg.MongoURIByPlatform["windows"] != "mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true" {
		t.Fatal("Windows必须显式使用新副本集，不能接入旧Lima转发或系统Mongo")
	}
}

// TestDevelopmentMongoRejectsForeignEndpoints 核对「意外接受外来端点」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDevelopmentMongoRejectsForeignEndpoints(t *testing.T) {
	// initial 保存由加载取得的初始值，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	initial, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// data 保存由读取文件取得的数据。
	data, err := os.ReadFile("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// invalid 接收遍历文本列表时当前的元素。
	for _, invalid := range []string{
		"mongodb://127.0.0.1:27017/", "mongodb://localhost:27119/?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/?replicaSet=foreign&directConnection=true",
		"mongodb://user:password@127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/user_database?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=false",
	} {
		// raw 记录原始值，使用按文本索引的任意类型的数据映射表示。
		var raw map[string]any
		if err = json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		raw["fixture_file"] = filepath.Join(initial.Root, initial.FixtureFile)
		if runtime.GOOS == "windows" {
			raw["mongo_uri_by_platform"].(map[string]any)["windows"] = invalid
		} else {
			raw["mongo_uri"] = invalid
		}
		// encoded 保存由序列化取得的“encoded”，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
		encoded, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		// path 保存由加入取得的路径。
		path := filepath.Join(t.TempDir(), "server.json")
		if err = os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatalf("意外接受外来端点：%s", invalid)
		}
	}
}
