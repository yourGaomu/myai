#!/bin/sh

set -eu

# The script is intended to live under the deployed project directory:
# /opt/myai/deploy/update-myai.sh
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ -n "${MYAI_PROJECT_DIR:-}" ]; then
  PROJECT_DIR=$MYAI_PROJECT_DIR
elif [ -f "$SCRIPT_DIR/compose.prod.yaml" ]; then
  # Also support copying this script next to compose.prod.yaml on the server.
  PROJECT_DIR=$SCRIPT_DIR
else
  PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
fi
COMPOSE_FILE="${MYAI_COMPOSE_FILE:-$PROJECT_DIR/compose.prod.yaml}"
ENV_FILE="${MYAI_ENV_FILE:-$PROJECT_DIR/.env}"

if [ ! -f "$COMPOSE_FILE" ]; then
  printf 'compose file not found: %s\n' "$COMPOSE_FILE" >&2
  exit 1
fi

cd "$PROJECT_DIR"

if [ -f "$ENV_FILE" ]; then
  compose() {
    docker compose --project-directory "$PROJECT_DIR" --file "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
  }
else
  compose() {
    docker compose --project-directory "$PROJECT_DIR" --file "$COMPOSE_FILE" "$@"
  }
fi

services="relay"
profile_args=""

if [ "${MYAI_UPDATE_AGENT:-true}" = "true" ]; then
  profile_args="$profile_args --profile agent"
  services="$services agent"
fi

if [ "${MYAI_UPDATE_SHORTENER:-true}" = "true" ]; then
  profile_args="$profile_args --profile shortener"
  services="$services url-shortener"
fi

printf 'Project: %s\n' "$PROJECT_DIR"
printf 'Services: %s\n' "$services"

# Validate interpolation and the active Compose profiles before pulling images.
compose $profile_args config --quiet

# Pull only application images. Databases and other infrastructure are not part
# of compose.prod.yaml, so they are never restarted by this script.
compose $profile_args pull $services
compose $profile_args up -d --force-recreate $services

compose $profile_args ps
