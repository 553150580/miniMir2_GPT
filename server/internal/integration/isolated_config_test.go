package integration

import (
	"errors"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"minimir/internal/devconfig"
)

// isolatedMongoURI 约束联机验收只接受独立副本集控制器生成的地址形状。
var isolatedMongoURI = regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`)

// loadIntegrationConfig 保留开发装配校验，允许在测试进程中替换已核验的独立数据库地址。
func loadIntegrationConfig() (devconfig.Config, error) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		return cfg, err
	}
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if uri == "" {
		return cfg, nil
	}
	if !isolatedMongoURI.MatchString(uri) {
		return cfg, errors.New("联机验收只接受独立副本集控制器生成的回环地址")
	}
	endpoint, err := url.Parse(uri)
	if err != nil {
		return cfg, err
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	if err != nil || port < 1024 || port == 27118 || port == 27119 {
		return cfg, errors.New("独立验收数据库端口无效或占用试玩端口")
	}
	cfg.MongoURI = uri
	return cfg, nil
}
