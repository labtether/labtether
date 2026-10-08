#!/usr/bin/env bash
# Inventory API smoke contracts; invoked after main safety/cleanup setup.

smoke_check_inventory() {
  # 10) asset heartbeat and inventory endpoints
  body=""
  status=""
  run_request body status POST "$API_BASE/assets/heartbeat" '{"asset_id":"'"$SMOKE_NODE_ID"'","type":"host","name":"Smoke Node '"$SMOKE_RUN_TOKEN"'","source":"smoke-test","status":"online","platform":"linux"}'
  assert_equal "POST /assets/heartbeat" "202" "$status"
  if [[ "$status" == "202" ]]; then
    smoke_register_cleanup DELETE "/assets/${SMOKE_NODE_ID}" "asset ${SMOKE_NODE_ID}"
  fi

  body=""
  status=""
  smoke_group_code="SMK$(date +%s)$((RANDOM % 1000))"
  run_request body status POST "$API_BASE/groups" "{\"name\":\"${SMOKE_GROUP_NAME}\",\"slug\":\"${smoke_group_code}\",\"location\":\"Austin\",\"latitude\":30.2672,\"longitude\":-97.7431,\"geo_label\":\"Austin, TX\",\"status\":\"active\"}"
  assert_equal "POST /groups" "201" "$status"
  smoke_group_id=$(extract_json_string "id" "$body")
  if [[ -z "$smoke_group_id" ]]; then
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse smoke group id from response\n'
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created smoke group %s\n' "$smoke_group_id"
    smoke_register_cleanup DELETE "/groups/${smoke_group_id}" "group ${smoke_group_id}"
  fi

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status POST "$API_BASE/assets/heartbeat" "{\"asset_id\":\"$SMOKE_GROUP_NODE_ID\",\"type\":\"host\",\"name\":\"Smoke Group Node ${SMOKE_RUN_TOKEN}\",\"source\":\"smoke-test\",\"group_id\":\"${smoke_group_id}\",\"status\":\"online\",\"platform\":\"linux\",\"metadata\":{\"cpu_percent\":\"15.3\",\"memory_percent\":\"44.1\"}}"
    assert_equal "POST /assets/heartbeat (group scoped)" "202" "$status"
    if [[ "$status" == "202" ]]; then
      smoke_register_cleanup DELETE "/assets/${SMOKE_GROUP_NODE_ID}" "asset ${SMOKE_GROUP_NODE_ID}"
    fi
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/assets"
  assert_equal "GET /assets" "200" "$status"
  if [[ "$body" == *"${SMOKE_NODE_ID}"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] assets list includes smoke heartbeat asset\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] assets list missing smoke heartbeat asset\n'
  fi

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/assets?group_id=${smoke_group_id}"
    assert_equal "GET /assets?group_id" "200" "$status"
    if [[ "$body" == *"${SMOKE_GROUP_NODE_ID}"* && "$body" != *"${SMOKE_NODE_ID}"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] group filtered assets returned scoped node only\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] group filtered assets did not return expected scoped list\n'
    fi
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/metrics/overview?window=15m"
  assert_equal "GET /metrics/overview" "200" "$status"

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/metrics/overview?window=15m&group_id=${smoke_group_id}"
    assert_equal "GET /metrics/overview?group_id" "200" "$status"
    if [[ "$body" == *"${SMOKE_GROUP_NODE_ID}"* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] group filtered telemetry overview returned scoped node\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] group filtered telemetry overview missing scoped node\n'
    fi
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/metrics/assets/${SMOKE_NODE_ID}?window=15m&step=30s"
  assert_equal "GET /metrics/assets/{id}" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/logs/sources?limit=10"
  assert_equal "GET /logs/sources" "200" "$status"

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/logs/sources?limit=10&group_id=${smoke_group_id}"
    assert_equal "GET /logs/sources?group_id" "200" "$status"
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/logs/query?window=1h&limit=20"
  assert_equal "GET /logs/query" "200" "$status"

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/logs/query?window=1h&limit=20&group_id=${smoke_group_id}"
    assert_equal "GET /logs/query?group_id" "200" "$status"
  fi

  body=""
  status=""
  run_request body status POST "$API_BASE/logs/views" "{\"name\":\"Smoke Logs ${SMOKE_RUN_TOKEN}\",\"window\":\"1h\",\"level\":\"info\"}"
  assert_equal "POST /logs/views" "201" "$status"
  smoke_log_view_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_log_view_id" ]]; then
    smoke_register_cleanup DELETE "/logs/views/${smoke_log_view_id}" "log view ${smoke_log_view_id}"
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/queue/dead-letters?window=24h&limit=20"
  assert_equal "GET /queue/dead-letters" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/connectors/proxmox/actions"
  assert_equal "GET /connectors/{id}/actions" "200" "$status"

  body=""
  status=""
  smoke_action_target="$SMOKE_NODE_ID"
  if labtether_value_is_true "$ALLOW_REMOTE_EXEC"; then
    smoke_action_target="$REMOTE_EXEC_TARGET"
  fi
  action_payload=$(jq -cn --arg target "$smoke_action_target" '{type:"command",actor_id:"owner",target:$target,command:"uptime"}')
  run_request body status POST "$API_BASE/actions/execute" "$action_payload"
  assert_equal "POST /actions/execute" "202" "$status"
  action_run_id=$(extract_json_string "id" "$body")
  if [[ -z "$action_run_id" ]]; then
    action_run_id=$(extract_json_string "run_id" "$body")
  fi
  if [[ -z "$action_run_id" ]]; then
    action_run_id=$(extract_nested_json_id "run" "$body")
  fi
  if [[ -n "$action_run_id" ]]; then
    smoke_register_cleanup DELETE "/actions/runs/${action_run_id}" "action run ${action_run_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse action run id for cleanup\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/actions/runs?limit=10"
  assert_equal "GET /actions/runs" "200" "$status"

  body=""
  status=""
  update_plan_payload=$(jq -cn \
    --arg name "Smoke Plan ${SMOKE_RUN_TOKEN}" \
    --arg target "$smoke_action_target" \
    '{name:$name,targets:[$target],scopes:["os_packages"],default_dry_run:true}')
  run_request body status POST "$API_BASE/updates/plans" "$update_plan_payload"
  assert_equal "POST /updates/plans" "201" "$status"
  update_plan_id=$(extract_json_string "id" "$body")
  if [[ -z "$update_plan_id" ]]; then
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse update plan id from response\n'
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created update plan %s\n' "$update_plan_id"
    smoke_register_cleanup DELETE "/updates/plans/${update_plan_id}" "update plan ${update_plan_id}"
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/updates/plans?limit=10"
  assert_equal "GET /updates/plans" "200" "$status"

  if [[ -n "$update_plan_id" ]]; then
    body=""
    status=""
    run_request body status POST "$API_BASE/updates/plans/${update_plan_id}/execute" '{"actor_id":"owner","dry_run":true}'
    assert_equal "POST /updates/plans/{id}/execute" "202" "$status"
    update_run_id=$(extract_json_string "id" "$body")
    if [[ -z "$update_run_id" ]]; then
      update_run_id=$(extract_nested_json_id "run" "$body")
    fi
    if [[ -n "$update_run_id" ]]; then
      smoke_register_cleanup DELETE "/updates/runs/${update_run_id}" "update run ${update_run_id}"
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] could not parse update run id for cleanup\n'
    fi
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/updates/runs?limit=10"
  assert_equal "GET /updates/runs" "200" "$status"

  body=""
  status=""
  run_request body status GET "$API_BASE/groups/reliability?window=24h"
  assert_equal "GET /groups/reliability" "200" "$status"

  if [[ -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/groups/${smoke_group_id}/timeline?window=24h&limit=20"
    assert_equal "GET /groups/{id}/timeline" "200" "$status"

    body=""
    status=""
    run_request body status POST "$API_BASE/groups/${smoke_group_id}/maintenance-windows" '{"name":"Smoke Guardrail Window","start_at":"2000-01-01T00:00:00Z","end_at":"2100-01-01T00:00:00Z","suppress_alerts":true,"block_actions":true,"block_updates":true}'
    assert_equal "POST /groups/{id}/maintenance-windows" "201" "$status"
    smoke_maintenance_window_id=$(extract_json_string "id" "$body")
    if [[ -n "$smoke_maintenance_window_id" ]]; then
      smoke_register_cleanup DELETE "/groups/${smoke_group_id}/maintenance-windows/${smoke_maintenance_window_id}" "maintenance window ${smoke_maintenance_window_id}"
    fi

    body=""
    status=""
    run_request body status GET "$API_BASE/groups/${smoke_group_id}/maintenance-windows?active=true&limit=10"
    assert_equal "GET /groups/{id}/maintenance-windows" "200" "$status"

    body=""
    status=""
    run_request body status POST "$API_BASE/actions/execute" "{\"type\":\"command\",\"actor_id\":\"owner\",\"target\":\"${SMOKE_GROUP_NODE_ID}\",\"command\":\"echo blocked by maintenance\"}"
    assert_equal "POST /actions/execute blocked by maintenance" "423" "$status"

    body=""
    status=""
    run_request body status POST "$API_BASE/updates/plans" "{\"name\":\"Smoke Maintenance Plan ${SMOKE_RUN_TOKEN}\",\"targets\":[\"${SMOKE_GROUP_NODE_ID}\"],\"scopes\":[\"os_packages\"],\"default_dry_run\":true}"
    assert_equal "POST /updates/plans (maintenance target)" "201" "$status"
    maintenance_plan_id=$(extract_json_string "id" "$body")
    if [[ -n "$maintenance_plan_id" ]]; then
      smoke_register_cleanup DELETE "/updates/plans/${maintenance_plan_id}" "update plan ${maintenance_plan_id}"
      body=""
      status=""
      run_request body status POST "$API_BASE/updates/plans/${maintenance_plan_id}/execute" '{"actor_id":"owner","dry_run":true}'
      assert_equal "POST /updates/plans/{id}/execute blocked by maintenance" "423" "$status"
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] could not parse maintenance plan id from response\n'
    fi
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/settings/runtime"
  assert_equal "GET /settings/runtime" "200" "$status"
  if [[ "$body" == *"console.poll_interval_seconds"* ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] runtime settings payload includes console.poll_interval_seconds\n'
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] runtime settings payload missing console.poll_interval_seconds\n'
  fi

  if labtether_value_is_true "$ALLOW_GLOBAL_SETTINGS"; then
    if printf '%s' "$body" | jq -e '.overrides | has("console.poll_interval_seconds")' >/dev/null 2>&1; then
      runtime_restore_mode="patch"
      prior_poll_override=$(printf '%s' "$body" | jq -er '.overrides["console.poll_interval_seconds"]') || {
        log "Unable to snapshot runtime setting; refusing to mutate it"
        exit 1
      }
      runtime_restore_payload=$(jq -cn --arg value "$prior_poll_override" '{values:{"console.poll_interval_seconds":$value}}')
    else
      runtime_restore_mode="reset"
      runtime_restore_payload='{}'
    fi

    runtime_settings_mutated=1
    body=""
    status=""
    run_request body status PATCH "$API_BASE/settings/runtime" '{"values":{"console.poll_interval_seconds":"7"}}'
    assert_equal "PATCH /settings/runtime" "200" "$status"
    if [[ "$body" == *"\"override_value\":\"7\""* ]]; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] runtime setting override persisted\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] runtime setting override not reflected\n'
    fi
    if restore_runtime_settings; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] runtime setting restored exactly\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] runtime setting restoration failed\n'
    fi
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] runtime setting mutation skipped (safe default)\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/settings/retention"
  assert_equal "GET /settings/retention" "200" "$status"
  if labtether_value_is_true "$ALLOW_DESTRUCTIVE_RETENTION"; then
    retention_restore_payload=$(printf '%s' "$body" | jq -cer '.settings') || {
      log "Unable to snapshot retention settings; refusing destructive mutation"
      exit 1
    }
    retention_settings_mutated=1
    body=""
    status=""
    run_request body status POST "$API_BASE/settings/retention" '{"preset":"balanced"}'
    assert_equal "POST /settings/retention" "200" "$status"
    if restore_retention_settings; then
      PASS_COUNT=$((PASS_COUNT + 1))
      printf '  [PASS] retention settings restored exactly\n'
    else
      FAIL_COUNT=$((FAIL_COUNT + 1))
      printf '  [FAIL] retention settings restoration failed\n'
    fi
  else
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] retention mutation skipped (safe default)\n'
  fi
}
