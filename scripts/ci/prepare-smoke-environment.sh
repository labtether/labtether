#!/usr/bin/env bash
set -Eeuo pipefail
install -m 600 .env.example .env
ci_token="$(openssl rand -hex 32)"
echo "::add-mask::${ci_token}"
{
  printf 'POSTGRES_USER=labtether\n'
  printf 'POSTGRES_DB=labtether\n'
  printf 'POSTGRES_PASSWORD=ci-smoke-pw\n'
  printf 'LABTETHER_OWNER_TOKEN=%s\n' "${ci_token}"
  printf 'LABTETHER_API_TOKEN=%s\n' "${ci_token}"
  printf 'LABTETHER_ADMIN_PASSWORD=ci-admin-pw\n'
  printf 'LABTETHER_ENCRYPTION_KEY=%s\n' "$(openssl rand -base64 32)"
  printf 'LABTETHER_ALLOW_LEGACY_SHARED_AGENT_AUTH=false\n'
} >> .env
printf 'LABTETHER_API_TOKEN=%s\n' "${ci_token}" >> "${GITHUB_ENV}"
