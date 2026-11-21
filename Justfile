export BRANCH := `git rev-parse --abbrev-ref HEAD`
export COMMIT := `git rev-parse --short HEAD`

build *ARGS:
    docker buildx bake \
        -f docker-bake.hcl \
        {{ ARGS }}

push *ARGS: (build "--push" ARGS)

bin:
    CGO_ENABLED=0 GOOS=linux go build     -trimpath     -ldflags="-s -w" -o ./build/storcli_exporter
