#!/usr/bin/env bash
# Automation API smoke contracts; invoked after main safety/cleanup setup.

smoke_check_automation() {
  # ── Stream 4: Synthetic Checks, Group Profiles, Failover Pairs, Hub Collectors ──

  log "\nSynthetic checks smoke checks"

  # Create synthetic check
  body=""
  status=""
  smoke_synthetic_target="${SYNTHETIC_TARGET:-https://smoke.invalid/healthz}"
  synthetic_payload=$(jq -cn \
    --arg name "Smoke HTTP Check ${SMOKE_RUN_TOKEN}" \
    --arg target "$smoke_synthetic_target" \
    --argjson enabled "$outbound_enabled" \
    '{name:$name,check_type:"http",target:$target,interval_seconds:60,enabled:$enabled}')
  run_request body status POST "$API_BASE/synthetic-checks" "$synthetic_payload"
  assert_equal "POST /synthetic-checks" "201" "$status"
  smoke_check_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_check_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created synthetic check %s\n' "$smoke_check_id"
    smoke_register_cleanup DELETE "/synthetic-checks/${smoke_check_id}" "synthetic check ${smoke_check_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse synthetic check id\n'
  fi

  # List synthetic checks
  body=""
  status=""
  run_request body status GET "$API_BASE/synthetic-checks?limit=10"
  assert_equal "GET /synthetic-checks" "200" "$status"

  # Get synthetic check results
  if [[ -n "${smoke_check_id:-}" ]]; then
    body=""
    status=""
    run_request body status GET "$API_BASE/synthetic-checks/${smoke_check_id}/results?limit=10"
    assert_equal "GET /synthetic-checks/{id}/results" "200" "$status"

    # Delete synthetic check
    body=""
    status=""
    run_request body status DELETE "$API_BASE/synthetic-checks/${smoke_check_id}"
    assert_equal "DELETE /synthetic-checks/{id}" "200" "$status"
  fi

  log "\nSite profiles smoke checks"

  body=""
  status=""
  run_request body status POST "$API_BASE/group-profiles" "{\"name\":\"Smoke Profile ${SMOKE_RUN_TOKEN}\",\"description\":\"Smoke test profile\",\"config\":{\"expected_asset_count\":2,\"required_platforms\":[\"linux\"],\"min_online_percent\":80}}"
  assert_equal "POST /group-profiles" "201" "$status"
  smoke_profile_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_profile_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created group profile %s\n' "$smoke_profile_id"
    smoke_register_cleanup DELETE "/group-profiles/${smoke_profile_id}" "group profile ${smoke_profile_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse group profile id\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/group-profiles?limit=10"
  assert_equal "GET /group-profiles" "200" "$status"

  if [[ -n "${smoke_profile_id:-}" && -n "${smoke_group_id:-}" ]]; then
    body=""
    status=""
    run_request body status POST "$API_BASE/group-profiles/${smoke_profile_id}/assign" "{\"group_id\":\"${smoke_group_id}\"}"
    assert_equal "POST /group-profiles/{id}/assign" "201" "$status"
  fi

  if [[ -n "${smoke_profile_id:-}" ]]; then
    body=""
    status=""
    run_request body status DELETE "$API_BASE/group-profiles/${smoke_profile_id}"
    assert_equal "DELETE /group-profiles/{id}" "200" "$status"
  fi

  log "\nSite failover pairs smoke checks"

  if [[ -n "${smoke_group_id:-}" ]]; then
    # Create a second group for failover pairing
    body=""
    status=""
    smoke_backup_site_code="SMB$(date +%s)$((RANDOM % 1000))"
    run_request body status POST "$API_BASE/groups" "{\"name\":\"${SMOKE_BACKUP_GROUP_NAME}\",\"slug\":\"${smoke_backup_site_code}\",\"location\":\"Denver\",\"latitude\":39.7392,\"longitude\":-104.9903,\"geo_label\":\"Denver, CO\",\"status\":\"active\"}"
    assert_equal "POST /groups (backup)" "201" "$status"
    smoke_backup_group_id=$(extract_json_string "id" "$body")
    if [[ -n "$smoke_backup_group_id" ]]; then
      smoke_register_cleanup DELETE "/groups/${smoke_backup_group_id}" "group ${smoke_backup_group_id}"
    fi

    if [[ -n "$smoke_backup_group_id" ]]; then
      body=""
      status=""
      run_request body status POST "$API_BASE/group-failover-pairs" "{\"primary_group_id\":\"${smoke_group_id}\",\"backup_group_id\":\"${smoke_backup_group_id}\"}"
      assert_equal "POST /group-failover-pairs" "201" "$status"
      smoke_failover_id=$(extract_json_string "id" "$body")
      if [[ -n "$smoke_failover_id" ]]; then
        PASS_COUNT=$((PASS_COUNT + 1))
        printf '  [PASS] created failover pair %s\n' "$smoke_failover_id"
        smoke_register_cleanup DELETE "/group-failover-pairs/${smoke_failover_id}" "failover pair ${smoke_failover_id}"
      else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        printf '  [FAIL] could not parse failover pair id\n'
      fi

      body=""
      status=""
      run_request body status GET "$API_BASE/group-failover-pairs?limit=10"
      assert_equal "GET /group-failover-pairs" "200" "$status"

      if [[ -n "${smoke_failover_id:-}" ]]; then
        body=""
        status=""
        run_request body status POST "$API_BASE/group-failover-pairs/${smoke_failover_id}/check-readiness"
        assert_equal "POST /group-failover-pairs/{id}/check-readiness" "200" "$status"

        body=""
        status=""
        run_request body status DELETE "$API_BASE/group-failover-pairs/${smoke_failover_id}"
        assert_equal "DELETE /group-failover-pairs/{id}" "200" "$status"
      fi
    fi
  fi

  log "\nHub collectors smoke checks"

  body=""
  status=""
  smoke_ssh_target="${SSH_TARGET:-smoke.invalid}"
  collector_payload=$(jq -cn \
    --arg asset "$SMOKE_NODE_ID" \
    --arg host "$smoke_ssh_target" \
    --argjson enabled "$outbound_enabled" \
    '{asset_id:$asset,collector_type:"ssh",config:{host:$host,user:"labtether-smoke",script:"uname -a"},interval_seconds:300,enabled:$enabled}')
  run_request body status POST "$API_BASE/hub-collectors" "$collector_payload"
  assert_equal "POST /hub-collectors" "201" "$status"
  smoke_collector_id=$(extract_json_string "id" "$body")
  if [[ -n "$smoke_collector_id" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf '  [PASS] created hub collector %s\n' "$smoke_collector_id"
    smoke_register_cleanup DELETE "/hub-collectors/${smoke_collector_id}" "hub collector ${smoke_collector_id}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf '  [FAIL] could not parse hub collector id\n'
  fi

  body=""
  status=""
  run_request body status GET "$API_BASE/hub-collectors?limit=10"
  assert_equal "GET /hub-collectors" "200" "$status"

  if [[ -n "${smoke_collector_id:-}" ]]; then
    body=""
    status=""
    run_request body status DELETE "$API_BASE/hub-collectors/${smoke_collector_id}"
    assert_equal "DELETE /hub-collectors/{id}" "200" "$status"
  fi
}
