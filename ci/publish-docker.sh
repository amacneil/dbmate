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

if [[ "$GITHUB_REF" = refs/tags/v* ]]; then
    # Publish major/minor/patch/latest version tags
    ver=${GITHUB_REF#refs/tags/v}

    publish "$ver"          # e.g. `1.2.3`
    publish "${ver%.*}"     # e.g. `1.2`
    publish "${ver%%.*}"    # e.g. `1`
    publish "latest"
else
    # Publish branch
    publish "${GITHUB_REF##*/}"
fi

# Clear credentials
rm -f ~/.docker/config.json
