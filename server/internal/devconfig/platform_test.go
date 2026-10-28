package devconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDevelopmentMongoPlatformEndpoint(t *testing.T) {
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

func TestDevelopmentMongoRejectsForeignEndpoints(t *testing.T) {
	initial, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		"mongodb://127.0.0.1:27017/", "mongodb://localhost:27119/?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/?replicaSet=foreign&directConnection=true",
		"mongodb://user:password@127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/user_database?replicaSet=minimirWinDev&directConnection=true",
		"mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=false",
	} {
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
		encoded, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "server.json")
		if err = os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatalf("意外接受外来端点：%s", invalid)
		}
	}
}
