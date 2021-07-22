#!/bin/bash
# Tag and publish Docker image

set -euo pipefail

echo "$DOCKERHUB_TOKEN" | (set -x && docker login --username "$DOCKERHUB_USERNAME" --password-stdin)
echo "$GHCR_TOKEN" | (set -x && docker login ghcr.io --username "$GHCR_USERNAME" --password-stdin)

# Tag and push docker image
function docker_push {
    src=$1
    dst=$2
    echo  # newline

    (
        set -x
        docker tag "$src" "$dst"
        docker push "$dst"
    )
}

# Publish image to both Docker Hub and GitHub Container Registry
function publish {
    tag=$1
    docker_push "$SRC_IMAGE" "$DOCKERHUB_IMAGE:$tag"
    docker_push "$SRC_IMAGE" "$GHCR_IMAGE:$tag"
}

# Publish current branch/tag (e.g. `main` or `v1.2.3`)
ver=${GITHUB_REF##*/}
publish "$ver"

# Publish major/minor/latest for version tags
if [[ "$GITHUB_REF" = refs/tags/v* ]]; then
    major_ver=${ver%%.*}  # e.g. `v1`
    publish "$major_ver"

    minor_ver=${ver%.*}  # e.g. `v1.2`
    publish "$minor_ver"

    publish "latest"
fi

# Clear credentials
rm -f ~/.docker/config.json
