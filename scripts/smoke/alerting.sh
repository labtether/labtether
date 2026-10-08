#!/usr/bin/env bash
# Alerting API smoke contracts; invoked after main safety/cleanup setup.

smoke_check_alerting() {
  # ── Stream 1: MVP Authentication ──

  log "\nAuth smoke checks"

  body=""
  status=""
  run_request body status POST "$API_BASE/auth/login" '{"username":"admin","password":"wrong-password"}' 0
  assert_equal "POST /auth/login bad creds" "401" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/auth/me" '' 0
  assert_equal "GET /auth/me no auth" "401" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/auth/me"
  assert_equal "GET /auth/me bearer token" "200" "$status"

  body=""
  status=""
  run_request body status POST "$API_BASE/auth/logout" '' 0
  assert_equal "POST /auth/logout" "200" "$status"

  # ── Stream 2: Alert Instances, Silences, Notifications, Routes ──

  log "\nAlert instances & silences smoke checks"

  body=""
  status=""
  run_request body status GET "$API_BASE/alerts/instances?limit=10"
  assert_equal "GET /alerts/instances" "200" "$status"

  body=""
  status=""
  run_request body status POST "$API_BASE/alerts/silences" "{\"matchers\":{\"asset_id\":\"${SMOKE_NODE_ID}\"},\"reason\":\"smoke test ${SMOKE_RUN_TOKEN}\",\"starts_at\":\"2000-01-01T00:00:00Z\",\"ends_at\":\"2100-01-01T00:00:00Z\"}"
  assert_equal "POST /alerts/silences" "201" "$status"
  smoke_silence_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_silence_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created silence %s\n' "$smoke_silence_id"
    smoke_register_cleanup DELETE "/alerts/silences/${smoke_silence_id}" "alert silence ${smoke_silence_id}"

    body=""
    status=""
    run_request body status GET "$API_BASE/alerts/silences?limit=10"
    assert_equal "GET /alerts/silences" "200" "$status"

    body=""
    status=""
    run_request body status DELETE "$API_BASE/alerts/silences/${smoke_silence_id}"
    assert_equal "DELETE /alerts/silences/{id}" "200" "$status"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse silence id\n'
  fi

  log "\nNotification channels & routes smoke checks"

  body=""
  status=""
  outbound_enabled=false
  smoke_webhook_target="${WEBHOOK_TARGET:-https://smoke.invalid/hook}"
  if labtether_value_is_true "$ALLOW_OUTBOUND"; then
    outbound_enabled=true
  fi
  channel_payload=$(jq -cn \
    --arg name "Smoke Webhook ${SMOKE_RUN_TOKEN}" \
    --arg url "$smoke_webhook_target" \
    --argjson enabled "$outbound_enabled" \
    '{name:$name,type:"webhook",config:{url:$url},enabled:$enabled}')
  run_request body status POST "$API_BASE/notifications/channels" "$channel_payload"
  assert_equal "POST /notifications/channels" "201" "$status"
  smoke_channel_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_channel_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created channel %s\n' "$smoke_channel_id"
    smoke_register_cleanup DELETE "/notifications/channels/${smoke_channel_id}" "notification channel ${smoke_channel_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse channel id\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/notifications/channels?limit=10"
  assert_equal "GET /notifications/channels" "200" "$status"

  body=""
  status=""
  route_payload=$(jq -cn \
    --arg name "Smoke Route ${SMOKE_RUN_TOKEN}" \
    --arg channel "${smoke_channel_id:-none}" \
    --arg asset "$SMOKE_NODE_ID" \
    --argjson enabled "$outbound_enabled" \
    '{name:$name,matchers:{asset_id:$asset},channel_ids:[$channel],enabled:$enabled}')
  run_request body status POST "$API_BASE/alerts/routes" "$route_payload"
  assert_equal "POST /alerts/routes" "201" "$status"
  smoke_route_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_route_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created route %s\n' "$smoke_route_id"
    smoke_register_cleanup DELETE "/alerts/routes/${smoke_route_id}" "alert route ${smoke_route_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse route id\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/alerts/routes?limit=10"
  assert_equal "GET /alerts/routes" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/notifications/history?limit=10"
  assert_equal "GET /notifications/history" "200" "$status"
}
