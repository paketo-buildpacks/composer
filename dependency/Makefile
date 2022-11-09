.PHONY: retrieve

retrieve:
	@cd retrieval; \
	go test -v && go run . \
		--buildpack_toml_path=$(buildpackTomlPath) \
		--output=$(output)
