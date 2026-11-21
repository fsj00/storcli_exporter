export BRANCH := `git rev-parse --abbrev-ref HEAD`
export COMMIT := `git rev-parse --short HEAD`

build *ARGS:
    docker buildx bake \
        -f docker-bake.hcl \
        {{ ARGS }}

push *ARGS: (build "--push" ARGS)
