.PHONY: check preview release-check test references environment protocol resources-check client-scene-test client-test client-build dev-start dev-stop dev-status server-integration-test development-resources-check

check:
	./tools/dev python3 tools/reference/inventory.py --check
	./tools/dev python3 tools/reference/prepare_local.py --check
	./tools/dev python3 tools/config/export_configs.py --check
	./tools/dev python3 tools/config/generate_protocol.py --check

preview:
	./tools/dev python3 tools/config/export_configs.py
	./tools/dev python3 tools/config/sync_bindings.py

release-check:
	./tools/dev python3 tools/config/export_configs.py --check --mode release

test: client-scene-test client-test
	./tools/dev python3 -m unittest discover -s tools/config -p 'test_*.py'
	./tools/dev go test ./...
	./tools/dev node tools/protocol/verify_client_pb.cjs
	./tools/dev node tools/config/verify_bindings.mjs

client-test:
	./tools/dev node client/tests/ui/compile_models.mjs
	./tools/dev node --test client/tests/ui/*.test.cjs client/tests/app/*.test.cjs

client-build:
	./tools/dev python3 tools/client/build.py

dev-start:
	./tools/dev python3 tools/environment/mongodb_dev.py start
	./tools/dev python3 tools/server/dev.py start
	./tools/dev python3 tools/client/dev.py start

dev-stop:
	./tools/dev python3 tools/client/dev.py stop
	./tools/dev python3 tools/server/dev.py stop

dev-status:
	./tools/dev python3 tools/server/dev.py status
	./tools/dev python3 tools/client/dev.py status

server-integration-test:
	./tools/dev python3 tools/server/test.py --mongo --race

development-resources-check:
	./tools/dev python3 tools/resources/build_development_assets.py --check
	./tools/dev python3 tools/resources/prepare_first_loop.py --check

client-scene-test:
	./tools/dev node tools/client/compile_scene_tests.mjs
	./tools/dev node --test client/tests/scene/*.test.cjs

references:
	./tools/dev python3 tools/reference/inventory.py
	./tools/dev python3 tools/reference/prepare_local.py

environment:
	./tools/dev python3 tools/environment/check.py

protocol:
	./tools/dev python3 tools/config/generate_protocol.py
	./tools/dev python3 tools/protocol/generate_go_pb.py
	./tools/dev node tools/protocol/generate_client_pb.cjs

resources-check:
	./tools/dev python3 tools/resources/inspect_wil.py --check
